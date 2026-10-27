package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/agents"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/session"
	"sharedmemory/internal/testutil"
)

func agentsRouter(t *testing.T) (*httptest.Server, uuid.UUID, string, uuid.UUID, string) {
	t.Helper()
	p := testutil.TestPool(t)
	acctA := testutil.NewAccount(t, p)
	acctB := testutil.NewAccount(t, p)
	authStore := auth.NewStore(p)
	keyA, err := authStore.CreateAPIKey(context.Background(), acctA, "agents-a")
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := authStore.CreateAPIKey(context.Background(), acctB, "agents-b")
	if err != nil {
		t.Fatal(err)
	}
	projects := project.NewStore(p)
	policies := policy.NewStore(p)
	h := httpapi.NewRouter(httpapi.Deps{
		Auth:     authStore,
		Agents:   agents.NewStore(p, authStore),
		Projects: projects,
		Policies: policies,
		Sessions: &session.Service{
			Pool: p, Objects: objectstore.NewMemory(), Queue: &queue.Recorder{},
			Policies: policies, Projects: projects,
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, acctA, keyA, acctB, keyB
}

func authReq(t *testing.T, method, url, key string, body any) *http.Request {
	t.Helper()
	var r *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		r, _ = http.NewRequest(method, url, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r, _ = http.NewRequest(method, url, nil)
	}
	r.Header.Set("Authorization", "Bearer "+key)
	return r
}

func TestAgents_IngestAutoAndList(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	ing := map[string]any{
		"agent": "cursor", "agent_path": "cursor", "role": "user", "content": "hello agents test",
		"skills": []string{}, "plugins": []string{}, "tools": []string{}, "model": "test",
	}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/ingest", keyA, ing))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("ingest status = %d", res.StatusCode)
	}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodGet, srv.URL+"/v1/agents", keyA, nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("agents status = %d", res.StatusCode)
	}
	var out struct {
		Groups []struct {
			Agent     string `json:"agent"`
			Instances []struct {
				Kind string `json:"kind"`
			} `json:"instances"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 1 || out.Groups[0].Agent != "cursor" || len(out.Groups[0].Instances) != 1 {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if out.Groups[0].Instances[0].Kind != agents.KindAuto {
		t.Fatalf("kind = %s", out.Groups[0].Instances[0].Kind)
	}
}

func TestAgents_InvalidConnectionIDOnIngest(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	ing := map[string]any{
		"agent": "cursor", "agent_path": "cursor", "role": "user", "content": "x",
		"connection_id": uuid.NewString(),
		"skills": []string{}, "plugins": []string{}, "tools": []string{},
	}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/ingest", keyA, ing))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d want 400", res.StatusCode)
	}
}

func TestAgents_TenantIsolation(t *testing.T) {
	srv, _, keyA, _, keyB := agentsRouter(t)
	ing := map[string]any{
		"agent": "codex", "agent_path": "codex", "role": "user", "content": "only A",
		"skills": []string{}, "plugins": []string{}, "tools": []string{},
	}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/ingest", keyA, ing))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("ingest = %d", res.StatusCode)
	}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodGet, srv.URL+"/v1/agents", keyB, nil))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Groups []any `json:"groups"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 0 {
		t.Fatalf("B should see no groups, got %d", len(out.Groups))
	}
}

func TestAgents_ClaimHistoryAndSessionFilter(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	ing := map[string]any{
		"agent": "cursor", "agent_path": "cursor", "role": "user", "content": "claim me",
		"skills": []string{}, "plugins": []string{}, "tools": []string{},
	}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/ingest", keyA, ing))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("ingest: %v status=%d", err, res.StatusCode)
	}
	createBody := map[string]any{"agent": "cursor", "label": "Claim test", "issue_dedicated_key": false}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections", keyA, createBody))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("create conn: %v status=%d", err, res.StatusCode)
	}
	var created struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/"+created.ConnectionID+"/claim-history", keyA, map[string]any{}))
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("claim: %v status=%d", err, res.StatusCode)
	}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodGet, srv.URL+"/v1/sessions?connection_id="+created.ConnectionID, keyA, nil))
	if err != nil {
		t.Fatal(err)
	}
	var sessions []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 1 {
		t.Fatalf("expected sessions for connection, got %d", len(sessions))
	}
}

func TestCreateConnectionClaimHistory(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	defer srv.Close()
	ing := map[string]any{
		"agent": "cursor", "agent_path": "cursor", "role": "user", "content": "orphan for create-claim",
		"skills": []string{}, "plugins": []string{}, "tools": []string{},
	}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/ingest", keyA, ing))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("ingest: %v status=%d", err, res.StatusCode)
	}
	body := map[string]any{"agent": "cursor", "label": "claim-on-create", "claim_history": true}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections", keyA, body))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("create with claim: %v status=%d", err, res.StatusCode)
	}
	var created struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodGet, srv.URL+"/v1/sessions?connection_id="+created.ConnectionID, keyA, nil))
	if err != nil {
		t.Fatal(err)
	}
	var sessions []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 1 {
		t.Fatalf("expected claimed session on create, got %d", len(sessions))
	}
}
