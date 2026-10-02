package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sharedmemory/internal/app"
	"sharedmemory/internal/config"
	"sharedmemory/internal/retention"
	"sharedmemory/internal/vault"
)

func main() {
	path := ".env"
	if v := os.Getenv("SHARED_MEMORY_ENV"); v != "" {
		path = v
	}
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	go housekeep(ctx, vault.NewStore(a.Pool, nil), &retention.Purger{Pool: a.Pool, Objects: a.Objects})

	consumer, _ := os.Hostname()
	if consumer == "" {
		consumer = "worker"
	}
	log.Println("worker consuming as", consumer)
	if err := a.Streams.Consume(ctx, consumer, a.Worker.Handle); err != nil {
		log.Fatal(err)
	}
}

// housekeep hourly drops unanswered vault candidates (encrypted secrets the
// user never consented to keep), expired artifact candidates, and evidence
// older than each account's retention_days.
func housekeep(ctx context.Context, s *vault.Store, p *retention.Purger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := s.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
			log.Printf("purge vault candidates: %v", err)
		}
		st, err := p.Run(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("retention purge: %v", err)
		}
		if st.Sessions > 0 || st.ArtifactCandidates > 0 {
			log.Printf("retention purge: %d sessions, %d attachments, %d artifact candidates", st.Sessions, st.Attachments, st.ArtifactCandidates)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
