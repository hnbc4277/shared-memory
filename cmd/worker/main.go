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

	go purgeExpiredCandidates(ctx, vault.NewStore(a.Pool, nil))

	consumer, _ := os.Hostname()
	if consumer == "" {
		consumer = "worker"
	}
	log.Println("worker consuming as", consumer)
	if err := a.Streams.Consume(ctx, consumer, a.Worker.Handle); err != nil {
		log.Fatal(err)
	}
}

// purgeExpiredCandidates drops unanswered vault candidates (encrypted secrets
// the user never consented to keep) once they expire.
func purgeExpiredCandidates(ctx context.Context, s *vault.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := s.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
			log.Printf("purge vault candidates: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
