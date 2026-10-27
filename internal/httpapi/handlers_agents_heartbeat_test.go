package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAgentsHeartbeat_DedicatedKey(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	createBody := map[string]any{
		"agent": "cursor", "label": "HB dedicated", "issue_dedicated_key": true,
	}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections", keyA, createBody))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("create conn: %v status=%d", err, res.StatusCode)
	}
	var created struct {
		ConnectionID string `json:"connection_id"`
		APIKey       string `json:"api_key"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if created.APIKey == "" {
		t.Fatal("expected dedicated api_key")
	}

	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/heartbeat", created.APIKey, map[string]any{"client": "mcp"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("heartbeat status = %d want 204", res.StatusCode)
	}
	res.Body.Close()

	res, err = http.DefaultClient.Do(authReq(t, http.MethodGet, srv.URL+"/v1/agents", keyA, nil))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Groups []struct {
			Instances []struct {
				ID            string `json:"id"`
				ProcessOnline bool   `json:"process_online"`
			} `json:"instances"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	found := false
	for _, g := range out.Groups {
		for _, inst := range g.Instances {
			if inst.ID == created.ConnectionID {
				found = true
				if !inst.ProcessOnline {
					t.Fatalf("expected process_online true for %s", created.ConnectionID)
				}
			}
		}
	}
	if !found {
		t.Fatalf("connection %s not in agents list", created.ConnectionID)
	}
}

func TestAgentsHeartbeat_AccountKeyRequiresConnectionID(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	createBody := map[string]any{"agent": "cursor", "label": "HB account", "issue_dedicated_key": false}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections", keyA, createBody))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("create conn: %v status=%d", err, res.StatusCode)
	}
	var created struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/heartbeat", keyA, map[string]any{"client": "mcp"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("heartbeat without connection_id status = %d want 400", res.StatusCode)
	}
	res.Body.Close()

	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/heartbeat", keyA, map[string]any{
		"client": "mcp", "connection_id": created.ConnectionID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("heartbeat with connection_id status = %d want 204", res.StatusCode)
	}
	res.Body.Close()
}

func TestAgentsHeartbeat_DedicatedKeyMismatch(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	mk := func(label string) (connID, dedicatedKey string) {
		t.Helper()
		res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections", keyA, map[string]any{
			"agent": "cursor", "label": label, "issue_dedicated_key": true,
		}))
		if err != nil || res.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: %v status=%d", label, err, res.StatusCode)
		}
		var created struct {
			ConnectionID string `json:"connection_id"`
			APIKey       string `json:"api_key"`
		}
		if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return created.ConnectionID, created.APIKey
	}
	connA, keyDedicated := mk("HB A")
	connB, _ := mk("HB B")
	if connA == connB {
		t.Fatal("expected distinct connections")
	}

	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/heartbeat", keyDedicated, map[string]any{
		"connection_id": connB,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatch heartbeat status = %d want 403", res.StatusCode)
	}
	res.Body.Close()
}

func TestAgentsHeartbeat_Revoked(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	createBody := map[string]any{"agent": "cursor", "label": "HB revoke", "issue_dedicated_key": false}
	res, err := http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections", keyA, createBody))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("create conn: %v status=%d", err, res.StatusCode)
	}
	var created struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/heartbeat", keyA, map[string]any{
		"connection_id": created.ConnectionID,
	}))
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("pre-revoke heartbeat: %v status=%d", err, res.StatusCode)
	}
	res.Body.Close()

	res, err = http.DefaultClient.Do(authReq(t, http.MethodDelete, srv.URL+"/v1/agents/connections/"+created.ConnectionID, keyA, nil))
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: %v status=%d", err, res.StatusCode)
	}
	res.Body.Close()

	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/agents/connections/heartbeat", keyA, map[string]any{
		"connection_id": created.ConnectionID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("post-revoke heartbeat status = %d want 404", res.StatusCode)
	}
	res.Body.Close()
}
