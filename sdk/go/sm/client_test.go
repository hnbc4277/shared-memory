package sm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIngestAndRecall(t *testing.T) {
	var gotAuth, gotIngestPath string
	var ingestBody IngestRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/ingest":
			gotIngestPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &ingestBody); err != nil {
				t.Errorf("ingest body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"session_id":"s1","message_id":"m1","seq":1,"warnings":[],"prompts":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/recall":
			_, _ = w.Write([]byte(`{"profile":"p","running_summary":"","hits":[],"context":"ctx","inject_enabled":true,"warnings":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			_, _ = w.Write([]byte(`[{"id":"s1","agent":"cursor","title":"t"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/s1":
			_, _ = w.Write([]byte(`{"session":{"id":"s1"},"messages":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/candidates":
			_, _ = w.Write([]byte(`{"vault":[],"artifacts":[]}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/vault/candidates/"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/device/code":
			_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"ABCD1234","verification_uri":"http://x/device","verification_uri_complete":"http://x/device?user_code=ABCD1234","expires_in":600,"interval":5}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/devices/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, "test-key")
	ctx := context.Background()

	ing, err := c.Ingest(ctx, IngestRequest{
		Agent: "cursor", Skills: []string{}, Plugins: []string{}, Tools: []string{},
		Role: "user", Content: "hi", ExternalSessionID: "cursor:1",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotIngestPath != "/v1/ingest" {
		t.Fatalf("path = %q", gotIngestPath)
	}
	if ingestBody.Content != "hi" || ingestBody.ExternalSessionID != "cursor:1" {
		t.Fatalf("body = %+v", ingestBody)
	}
	if ing.SessionID != "s1" || ing.MessageID != "m1" {
		t.Fatalf("ingest result = %+v", ing)
	}

	rec, err := c.Recall(ctx, RecallRequest{Query: "prefs", TopK: 5})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if rec.Context != "ctx" || !rec.InjectEnabled {
		t.Fatalf("recall = %+v", rec)
	}

	sessions, err := c.ListSessions(ctx, ListSessionsFilter{Agent: "cursor", Limit: 10})
	if err != nil || len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("ListSessions: %v %+v", err, sessions)
	}

	detail, err := c.GetSession(ctx, "s1")
	if err != nil || detail.Session.ID != "s1" {
		t.Fatalf("GetSession: %v %+v", err, detail)
	}

	cands, err := c.ListCandidates(ctx)
	if err != nil || cands.Vault == nil || cands.Artifacts == nil {
		t.Fatalf("ListCandidates: %v %+v", err, cands)
	}

	if _, err := c.ResolveCandidate(ctx, "vault", "c1", "reject"); err != nil {
		t.Fatalf("ResolveCandidate: %v", err)
	}

	code, err := c.CreateDeviceCode(ctx, DeviceCodeRequest{ClientName: "daemon", Agent: "cursor"})
	if err != nil || code.UserCode != "ABCD1234" {
		t.Fatalf("CreateDeviceCode: %v %+v", err, code)
	}
	if err := c.Heartbeat(ctx, "0.1.0", 0); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := NewClient(srv.URL, "bad").Recall(context.Background(), RecallRequest{Query: "x"})
	ae, ok := err.(*APIError)
	if !ok || ae.Status != 401 {
		t.Fatalf("want APIError 401, got %v", err)
	}
}
