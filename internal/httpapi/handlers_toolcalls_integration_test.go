//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/session"
	"sharedmemory/internal/testutil"
)

func TestSessionDetailIncludesToolCalls(t *testing.T) {
	p := testutil.TestPool(t)
	acct := testutil.NewAccount(t, p)
	authStore := auth.NewStore(p)
	key, err := authStore.CreateAPIKey(context.Background(), acct, "tc")
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewStore(p)
	policies := policy.NewStore(p)
	pid, err := projects.Create(context.Background(), acct, "tc-proj")
	if err != nil {
		t.Fatal(err)
	}
	objects := objectstore.NewMemory()
	sessSvc := &session.Service{
		Pool: p, Objects: objects, Queue: &queue.Recorder{},
		Policies: policies, Projects: projects,
	}
	h := httpapi.NewRouter(httpapi.Deps{
		Auth: authStore, Projects: projects, Policies: policies, Sessions: sessSvc,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"project_id": pid.ID.String(), "agent": "cursor", "agent_path": "cursor", "role": "assistant",
		"skills": []string{}, "plugins": []string{}, "tools": []string{}, "model": "test",
		"content": "done", "tool_calls": []map[string]any{
			{"kind": "tool", "name": "shell", "output": "hello tool output"},
		},
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("ingest: %v status=%d", err, res.StatusCode)
	}
	var ing struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ing); err != nil {
		t.Fatal(err)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/sessions/"+ing.SessionID, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("get session: %v status=%d", err, res.StatusCode)
	}
	var detail struct {
		Messages []struct {
			ToolCalls []struct {
				Name   string `json:"name"`
				Output string `json:"output"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) == 0 || len(detail.Messages[0].ToolCalls) == 0 {
		t.Fatal("expected tool_calls on message")
	}
	if detail.Messages[0].ToolCalls[0].Name != "shell" {
		t.Fatalf("got %q", detail.Messages[0].ToolCalls[0].Name)
	}
}
