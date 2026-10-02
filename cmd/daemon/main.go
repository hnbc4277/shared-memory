package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"sharedmemory/internal/cred"
	"sharedmemory/internal/daemon"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("daemon: ")

	c, err := cred.Resolve()
	if err != nil {
		log.Fatalf("credentials: %v (run: smctl login)", err)
	}
	spoolDir, err := cred.SpoolDir()
	if err != nil {
		log.Fatal(err)
	}
	spool := daemon.NewSpool(spoolDir)
	srv := daemon.NewServer(c, spool)
	if v := os.Getenv("SHARED_MEMORY_DAEMON_ADDR"); v != "" {
		srv.ListenAddr = v
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Start(ctx); err != nil {
		log.Fatal(err)
	}
}
