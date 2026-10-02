package session_test

import (
	"encoding/json"
	"errors"
	"testing"

	"sharedmemory/internal/session"
)

func decode(t *testing.T, raw string) session.IngestMessageRequest {
	t.Helper()
	var req session.IngestMessageRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestValidateIngest(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"complete", `{"agent":"cursor","skills":[],"plugins":[],"tools":[],"role":"user","content":"hi"}`, true},
		{"sub agent", `{"agent":"xiaolongxia","agent_path":"xiaolongxia/researcher","skills":["s"],"plugins":["p"],"tools":[],"role":"assistant","content":"x"}`, true},
		{"missing agent", `{"skills":[],"plugins":[],"tools":[],"role":"user","content":"hi"}`, false},
		{"missing skills key", `{"agent":"cursor","plugins":[],"tools":[],"role":"user","content":"hi"}`, false},
		{"missing plugins key", `{"agent":"cursor","skills":[],"tools":[],"role":"user","content":"hi"}`, false},
		{"missing tools key", `{"agent":"cursor","skills":[],"plugins":[],"role":"user","content":"hi"}`, false},
		{"agent_path other agent", `{"agent":"cursor","agent_path":"codex/x","skills":[],"plugins":[],"tools":[],"role":"user","content":"hi"}`, false},
		{"agent with slash", `{"agent":"a/b","skills":[],"plugins":[],"tools":[],"role":"user","content":"hi"}`, false},
		{"bad role", `{"agent":"cursor","skills":[],"plugins":[],"tools":[],"role":"bot","content":"hi"}`, false},
		{"empty content", `{"agent":"cursor","skills":[],"plugins":[],"tools":[],"role":"user","content":"  "}`, false},
		{"bad session id", `{"session_id":"x","agent":"cursor","skills":[],"plugins":[],"tools":[],"role":"user","content":"hi"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := session.ValidateIngest(decode(t, tc.raw))
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && !errors.Is(err, session.ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

func TestNormalize_DefaultsAgentPathAndModel(t *testing.T) {
	req := session.Normalize(decode(t, `{"agent":"cursor","skills":[" a ",""],"plugins":[],"tools":[],"role":"user","content":"hi"}`))
	if req.AgentPath != "cursor" {
		t.Fatalf("agent_path = %q, want cursor", req.AgentPath)
	}
	if req.Model != session.UnknownModel {
		t.Fatalf("model = %q, want unknown", req.Model)
	}
	if got := *req.Skills; len(got) != 1 || got[0] != "a" {
		t.Fatalf("skills = %v", got)
	}
}
