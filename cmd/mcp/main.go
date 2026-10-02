package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"sharedmemory/internal/mcp"
)

// Configure in an MCP client, e.g.:
//
//	{"command": "shared-memory-mcp", "env": {"SHARED_MEMORY_API_KEY": "sm_...",
//	  "SHARED_MEMORY_API_URL": "http://127.0.0.1:18080", "SHARED_MEMORY_AGENT": "cursor"}}
func main() {
	log.SetOutput(os.Stderr)
	key := os.Getenv("SHARED_MEMORY_API_KEY")
	if key == "" {
		log.Fatal("SHARED_MEMORY_API_KEY is required")
	}
	base := os.Getenv("SHARED_MEMORY_API_URL")
	if base == "" {
		base = "http://127.0.0.1:18080"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s := &mcp.Server{
		Client:       &mcp.Client{BaseURL: base, APIKey: key},
		DefaultAgent: os.Getenv("SHARED_MEMORY_AGENT"),
		Version:      "0.1.0",
	}
	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
