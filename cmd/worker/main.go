package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/app"
	"sharedmemory/internal/config"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/reindex"
	"sharedmemory/internal/retention"
	"sharedmemory/internal/vault"
)

func main() {
	forceReindex := flag.Bool("reindex", false, "rebuild the search projection from Postgres and exit")
	flag.Parse()

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

	if *forceReindex {
		n, err := a.Reindexer.Run(ctx)
		if err != nil {
			log.Fatalf("reindex after %d docs: %v", n, err)
		}
		log.Printf("reindexed %d docs into %s", n, a.Reindexer.Collection)
		return
	}

	go housekeep(ctx, vault.NewStore(a.Pool, nil), &retention.Purger{Pool: a.Pool, Objects: a.Objects}, a.Reindexer, a.Pool, a.Config.RecallReceiptRetentionDays)

	consumer, _ := os.Hostname()
	if consumer == "" {
		consumer = "worker"
	}
	log.Println("worker consuming as", consumer)
	if err := a.Streams.Consume(ctx, consumer, a.Worker.Handle); err != nil {
		log.Fatal(err)
	}
}

// housekeep hourly drops unanswered vault candidates, expired evidence, and
// aged recall receipts. It also backfills a vector collection when needed.
func housekeep(ctx context.Context, s *vault.Store, p *retention.Purger, r *reindex.Reindexer, pool *pgxpool.Pool, receiptDays int) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		backfill(ctx, r)
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
		if n, err := recall.PurgeReceipts(ctx, pool, receiptDays); err != nil && ctx.Err() == nil {
			log.Printf("recall receipt purge: %v", err)
		} else if n > 0 {
			log.Printf("recall receipt purge: deleted %d rows (retention %dd)", n, receiptDays)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func backfill(ctx context.Context, r *reindex.Reindexer) {
	needed, err := r.Needed(ctx)
	if err != nil || !needed {
		if err != nil && ctx.Err() == nil {
			log.Printf("index state: %v", err)
		}
		return
	}
	n, err := r.Run(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("backfill %s stopped after %d docs, retrying next hour: %v", r.Collection, n, err)
		}
		return
	}
	log.Printf("backfilled %d docs into %s", n, r.Collection)
}
