//go:build integration

package queue_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"sharedmemory/internal/queue"
)

func TestStreams_EnqueueDrain(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:16379"})
	t.Cleanup(func() { rdb.Close() })
	stream := "sm:test:" + uuid.NewString()
	t.Cleanup(func() { rdb.Del(ctx, stream) })

	q := queue.NewStreams(rdb, stream, "g")
	want := queue.Job{Type: queue.JobMessage, AccountID: uuid.New(), Content: "hello"}
	if err := q.Enqueue(ctx, want); err != nil {
		t.Fatal(err)
	}
	var got []queue.Job
	n, err := q.Drain(ctx, "c1", func(_ context.Context, j queue.Job) error {
		got = append(got, j)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(got) != 1 || got[0].AccountID != want.AccountID || got[0].Content != "hello" {
		t.Fatalf("drain n=%d got=%+v", n, got)
	}
	if l, _ := rdb.XLen(ctx, stream).Result(); l != 0 {
		t.Fatalf("acked job must be deleted from stream, len=%d", l)
	}
}
