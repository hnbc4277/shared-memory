package main

import (
	"context"
	"io"
	"log"
	"os"
	"strings"

	"sharedmemory/internal/cred"
	"sharedmemory/internal/hook"
)

func main() {
	log.SetOutput(os.Stderr)
	surface := os.Getenv("SHARED_MEMORY_HOOK_SURFACE")
	if surface == "" && len(os.Args) > 1 {
		surface = os.Args[1]
	}
	if surface == "" {
		surface = "cursor"
	}

	api, err := resolveAPI()
	if err != nil {
		// Still emit valid JSON so host hooks do not fail hard.
		_, _ = io.WriteString(os.Stdout, "{}\n")
		log.Printf("api: %v", err)
		return
	}
	if err := hook.RunWithAgent(context.Background(), surface, os.Getenv("SHARED_MEMORY_AGENT"), api, os.Stdin, os.Stdout); err != nil {
		log.Printf("hook: %v", err)
		_, _ = io.WriteString(os.Stdout, "{}\n")
	}
}

func resolveAPI() (hook.API, error) {
	daemonURL := os.Getenv("SHARED_MEMORY_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = hook.DefaultDaemonURL
	}
	connID := strings.TrimSpace(os.Getenv(cred.EnvConnectionID))
	if hook.DaemonReachable(daemonURL, nil) {
		return &hook.HTTPAPI{BaseURL: daemonURL, ConnectionID: connID}, nil
	}
	c, err := cred.Resolve()
	if err != nil {
		return nil, err
	}
	if connID == "" {
		connID = c.ConnectionID
	}
	return &hook.HTTPAPI{BaseURL: c.APIURL, APIKey: c.APIKey, ConnectionID: connID}, nil
}

func init() {
	// Normalize surface aliases passed as argv.
	if len(os.Args) > 1 {
		os.Args[1] = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
}
