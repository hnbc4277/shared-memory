package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"sharedmemory/internal/smctl"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch os.Args[1] {
	case "login":
		apiURL := envOr("SHARED_MEMORY_API_URL", "")
		client := "smctl"
		agent := ""
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--api-url":
				i++
				if i < len(os.Args) {
					apiURL = os.Args[i]
				}
			case "--client-name":
				i++
				if i < len(os.Args) {
					client = os.Args[i]
				}
			case "--agent":
				i++
				if i < len(os.Args) {
					agent = os.Args[i]
				}
			}
		}
		if err := smctl.Login(ctx, apiURL, client, agent, os.Stdout); err != nil {
			fail(err)
		}
	case "install":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		smhook := flagValue(os.Args[3:], "--smhook")
		mcp := flagValue(os.Args[3:], "--mcp")
		switch strings.ToLower(os.Args[2]) {
		case "cursor":
			if err := smctl.InstallCursor(smhook, mcp); err != nil {
				fail(err)
			}
			fmt.Println("installed Cursor hooks, MCP snippet, and ~/.cursor/rules/shared-memory.mdc")
		case "codex":
			if err := smctl.InstallCodex(smhook, mcp); err != nil {
				fail(err)
			}
			fmt.Println("installed Codex hooks and shared-memory-mcp.json")
		default:
			usage()
			os.Exit(2)
		}
	case "status":
		if err := smctl.Status(ctx, os.Stdout); err != nil {
			fail(err)
		}
	case "logout":
		revoke := false
		for _, a := range os.Args[2:] {
			if a == "--revoke" {
				revoke = true
			}
		}
		if err := smctl.Logout(ctx, revoke); err != nil {
			fail(err)
		}
		fmt.Println("logged out")
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `smctl — Shared Memory local control

Usage:
  smctl login [--api-url URL] [--client-name NAME] [--agent AGENT]
  smctl install cursor [--smhook PATH] [--mcp PATH]
  smctl install codex [--smhook PATH] [--mcp PATH]
  smctl status
  smctl logout [--revoke]
`)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "smctl: %v\n", err)
	os.Exit(1)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func flagValue(args []string, name string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
