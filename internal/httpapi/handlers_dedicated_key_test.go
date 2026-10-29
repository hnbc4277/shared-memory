package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestIngest_DedicatedKeyInfersConnection(t *testing.T) {
	srv, _, keyA, _, _ := agentsRouter(t)
	createBody := map[string]any{
		"agent": "cursor", "label": "Dedicated smoke", "issue_dedicated_key": true,
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
	if created.APIKey == "" {
		t.Fatal("expected dedicated api_key")
	}
	ing := map[string]any{
		"agent": "cursor", "agent_path": "cursor", "role": "user", "content": "dedicated key bind",
		"skills": []string{}, "plugins": []string{}, "tools": []string{},
	}
	res, err = http.DefaultClient.Do(authReq(t, http.MethodPost, srv.URL+"/v1/ingest", created.APIKey, ing))
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("ingest with dedicated key: %v status=%d", err, res.StatusCode)
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
		t.Fatalf("expected session bound to connection %s", created.ConnectionID)
	}
}
