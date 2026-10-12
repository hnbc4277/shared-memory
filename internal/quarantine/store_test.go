//go:build integration

package quarantine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/quarantine"
	"sharedmemory/internal/testutil"
)

func TestStore_AddGetDelete(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	s := &quarantine.Store{Pool: pool}
	aid := testutil.NewAccount(t, pool)

	payload := map[string]any{
		"skills": []any{}, "plugins": []any{}, "tools": []any{},
		"role": "user", "content": "hi", "agent": "cursor",
	}
	if err := s.Add(ctx, &aid, "cursor", "cursor", "missing agent_path", payload); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, aid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Fatal("expected entry")
	}
	e, err := s.Get(ctx, aid, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if e.Agent != "cursor" || e.Reason == "" {
		t.Fatalf("entry = %+v", e)
	}
	var got map[string]any
	if err := json.Unmarshal(e.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["content"] != "hi" {
		t.Fatalf("payload = %v", got)
	}
	if err := s.UpdateReason(ctx, aid, e.ID, "still bad"); err != nil {
		t.Fatal(err)
	}
	e2, _ := s.Get(ctx, aid, e.ID)
	if e2.Reason != "still bad" {
		t.Fatalf("reason = %q", e2.Reason)
	}
	if err := s.Delete(ctx, aid, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, aid, e.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestStore_GetWrongAccount(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	s := &quarantine.Store{Pool: pool}
	a := testutil.NewAccount(t, pool)
	b := testutil.NewAccount(t, pool)
	_ = s.Add(ctx, &a, "x", "x", "bad", map[string]any{"role": "user"})
	list, _ := s.List(ctx, a, 1)
	if _, err := s.Get(ctx, b, list[0].ID); err == nil {
		t.Fatal("cross-account get must fail")
	}
	_ = uuid.Nil
}
