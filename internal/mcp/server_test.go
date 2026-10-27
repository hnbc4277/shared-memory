package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sharedmemory/internal/mcp"
)

type captured struct {
	method, path, auth string
	body               map[string]any
}

func run(t *testing.T, lines ...string) ([]map[string]any, []captured) {
	t.Helper()
	var calls []captured
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := captured{method: r.Method, path: r.URL.RequestURI(), auth: r.Header.Get("Authorization")}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &c.body)
		calls = append(calls, c)
		if strings.HasPrefix(r.URL.Path, "/v1/sessions/missing") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found: session"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()
	s := &mcp.Server{
		Client:         &mcp.Client{BaseURL: api.URL, APIKey: "sm_test"},
		DefaultAgent:   "cursor",
		HeartbeatEvery: -1,
		Version:        "test",
	}
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")), &out); err != nil {
		t.Fatal(err)
	}
	var resps []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		resps = append(resps, m)
	}
	return resps, calls
}

func TestServe_HandshakeAndToolList(t *testing.T) {
	resps, _ := run(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	)
	if len(resps) != 3 {
		t.Fatalf("notifications must not get responses; got %d responses", len(resps))
	}
	init := resps[0]["result"].(map[string]any)
	if init["protocolVersion"] != mcp.ProtocolVersion {
		t.Fatalf("initialize = %v", init)
	}
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"memory_ingest", "memory_recall", "memory_recall_status", "memory_list_sessions", "memory_get_session",
		"memory_list_candidates", "memory_resolve_candidate", "kb_add", "kb_list", "kb_get", "kb_delete", "agents_list"} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}
	if resps[2]["error"] == nil {
		t.Fatal("unknown method must return an error")
	}
}

func TestServe_ToolsProxyToAPI(t *testing.T) {
	resps, calls := run(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_ingest","arguments":{"skills":[],"plugins":[],"tools":[],"role":"user","content":"hi"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"memory_list_sessions","arguments":{"agent_path":"xiaolongxia/researcher","limit":5}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"memory_resolve_candidate","arguments":{"kind":"artifact","candidate_id":"c1","decision":"accept"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"memory_get_session","arguments":{"session_id":"missing"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"memory_recall_status","arguments":{"external_session_id":"ext-1","within_minutes":15}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"kb_add","arguments":{"title":"Spec","text":"# KB\nline"}}}`,
	)
	if len(calls) != 6 {
		t.Fatalf("api calls = %d", len(calls))
	}
	if calls[0].path != "/v1/ingest" || calls[0].body["agent"] != "cursor" || calls[0].auth != "Bearer sm_test" {
		t.Fatalf("ingest call = %+v", calls[0])
	}
	if !strings.HasPrefix(calls[1].path, "/v1/sessions?") || !strings.Contains(calls[1].path, "agent_path=xiaolongxia%2Fresearcher") ||
		!strings.Contains(calls[1].path, "limit=5") {
		t.Fatalf("list call = %s", calls[1].path)
	}
	if calls[2].method != "POST" || calls[2].path != "/v1/artifacts/candidates/c1/accept" {
		t.Fatalf("resolve call = %+v", calls[2])
	}
	last := resps[3]["result"].(map[string]any)
	if last["isError"] != true {
		t.Fatalf("API errors must surface as tool errors: %v", last)
	}
	if calls[4].method != "GET" || !strings.Contains(calls[4].path, "/v1/recall/receipts/check") ||
		!strings.Contains(calls[4].path, "external_session_id=ext-1") ||
		!strings.Contains(calls[4].path, "within_minutes=15") {
		t.Fatalf("recall_status call = %+v", calls[4])
	}
	if calls[5].method != "POST" || calls[5].path != "/v1/kb/documents" || calls[5].body["text"] != "# KB\nline" {
		t.Fatalf("kb_add call = %+v", calls[5])
	}
}

func TestServe_ConnectionHeartbeatTicker(t *testing.T) {
	var mu sync.Mutex
	var heartbeatPaths []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agents/connections/heartbeat" {
			mu.Lock()
			heartbeatPaths = append(heartbeatPaths, r.URL.Path)
			mu.Unlock()
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["client"] != "mcp" {
				t.Errorf("heartbeat client = %v", body["client"])
			}
			if body["connection_id"] != "conn-test-uuid" {
				t.Errorf("heartbeat connection_id = %v", body["connection_id"])
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &mcp.Server{
		Client:              &mcp.Client{BaseURL: api.URL, APIKey: "sm_test"},
		DefaultConnectionID: "conn-test-uuid",
		HeartbeatEvery:      20 * time.Millisecond,
		Version:             "test",
	}
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Serve(ctx, pr, io.Discard)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		mu.Lock()
		n := len(heartbeatPaths)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected at least one heartbeat POST, got %d", n)
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	_ = pw.Close()
	<-done
}
