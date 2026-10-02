package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultStream = "sm:jobs"
	DefaultGroup  = "sm-workers"
	MaxDeliveries = 5
	claimIdle     = 30 * time.Second
)

type Streams struct {
	rdb    *redis.Client
	stream string
	group  string
}

func NewStreams(rdb *redis.Client, stream, group string) *Streams {
	if stream == "" {
		stream = DefaultStream
	}
	if group == "" {
		group = DefaultGroup
	}
	return &Streams{rdb: rdb, stream: stream, group: group}
}

func (s *Streams) DeadStream() string { return s.stream + ":dead" }

func (s *Streams) Enqueue(ctx context.Context, job Job) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err := s.rdb.XAdd(ctx, &redis.XAddArgs{Stream: s.stream, Values: map[string]any{"job": string(b)}}).Err(); err != nil {
		return fmt.Errorf("enqueue: %w", err)
	}
	return nil
}

func (s *Streams) EnsureGroup(ctx context.Context) error {
	err := s.rdb.XGroupCreateMkStream(ctx, s.stream, s.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create group: %w", err)
	}
	return nil
}

type Handler func(ctx context.Context, job Job) error

// Consume blocks until ctx is cancelled. Successful jobs are acked and deleted;
// failed jobs stay pending, are reclaimed after claimIdle and moved to the dead
// stream after MaxDeliveries attempts.
func (s *Streams) Consume(ctx context.Context, consumer string, handle Handler) error {
	if err := s.EnsureGroup(ctx); err != nil {
		return err
	}
	lastClaim := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastClaim) > claimIdle/2 {
			if err := s.reclaim(ctx, consumer, handle); err != nil && ctx.Err() == nil {
				log.Printf("queue reclaim: %v", err)
			}
			lastClaim = time.Now()
		}
		res, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.group,
			Consumer: consumer,
			Streams:  []string{s.stream, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("queue read: %v", err)
			time.Sleep(time.Second)
			continue
		}
		for _, st := range res {
			for _, msg := range st.Messages {
				s.process(ctx, msg, handle)
			}
		}
	}
	return nil
}

// Drain processes everything currently queued and returns; used by tests and tooling.
func (s *Streams) Drain(ctx context.Context, consumer string, handle Handler) (int, error) {
	if err := s.EnsureGroup(ctx); err != nil {
		return 0, err
	}
	n := 0
	for {
		res, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: s.group, Consumer: consumer, Streams: []string{s.stream, ">"}, Count: 50, Block: -1,
		}).Result()
		if errors.Is(err, redis.Nil) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		got := 0
		for _, st := range res {
			for _, msg := range st.Messages {
				s.process(ctx, msg, handle)
				got++
			}
		}
		if got == 0 {
			return n, nil
		}
		n += got
	}
}

func (s *Streams) process(ctx context.Context, msg redis.XMessage, handle Handler) {
	raw, _ := msg.Values["job"].(string)
	var job Job
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		log.Printf("queue: dropping malformed job %s: %v", msg.ID, err)
		s.deadLetter(ctx, msg, "malformed: "+err.Error())
		return
	}
	if err := handle(ctx, job); err != nil {
		log.Printf("queue: job %s (%s) failed: %v", msg.ID, job.Type, err)
		return
	}
	s.ack(ctx, msg.ID)
}

func (s *Streams) ack(ctx context.Context, id string) {
	s.rdb.XAck(ctx, s.stream, s.group, id)
	s.rdb.XDel(ctx, s.stream, id)
}

func (s *Streams) deadLetter(ctx context.Context, msg redis.XMessage, reason string) {
	values := map[string]any{"reason": reason}
	for k, v := range msg.Values {
		values[k] = v
	}
	s.rdb.XAdd(ctx, &redis.XAddArgs{Stream: s.DeadStream(), Values: values})
	s.ack(ctx, msg.ID)
}

func (s *Streams) reclaim(ctx context.Context, consumer string, handle Handler) error {
	pending, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: s.stream, Group: s.group, Idle: claimIdle, Start: "-", End: "+", Count: 50,
	}).Result()
	if err != nil {
		return err
	}
	for _, p := range pending {
		msgs, err := s.rdb.XClaim(ctx, &redis.XClaimArgs{
			Stream: s.stream, Group: s.group, Consumer: consumer, MinIdle: claimIdle, Messages: []string{p.ID},
		}).Result()
		if err != nil || len(msgs) == 0 {
			continue
		}
		if p.RetryCount >= MaxDeliveries {
			s.deadLetter(ctx, msgs[0], fmt.Sprintf("exceeded %d deliveries", MaxDeliveries))
			continue
		}
		s.process(ctx, msgs[0], handle)
	}
	return nil
}
