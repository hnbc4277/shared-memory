//go:build integration

package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/testutil"
)

func uniqueEmail(prefix string) string {
	return prefix + "-" + uuid.NewString() + "@example.com"
}

func TestProjectList_IsolatedByAccount(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	as := auth.NewStore(pool)
	a1, err := as.CreateAccount(ctx, uniqueEmail("a1"), "password123")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := as.CreateAccount(ctx, uniqueEmail("a2"), "password123")
	if err != nil {
		t.Fatal(err)
	}
	ps := project.NewStore(pool)
	p1, err := ps.Create(ctx, a1, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	list2, err := ps.List(ctx, a2)
	if err != nil {
		t.Fatal(err)
	}
	if len(list2) != 0 {
		t.Fatalf("account2 must not see account1 projects, got %d", len(list2))
	}
	if _, err := ps.Get(ctx, a2, p1.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("account2 Get must be ErrNotFound, got %v", err)
	}
	pol, err := policy.NewStore(pool).Get(ctx, a1)
	if err != nil {
		t.Fatal(err)
	}
	if !pol.StoreTranscript || !pol.InjectEnabled {
		t.Fatalf("defaults must be true, got %+v", pol)
	}
}

func TestPolicy_UpdateRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	a, err := auth.NewStore(pool).CreateAccount(ctx, uniqueEmail("pol"), "password123")
	if err != nil {
		t.Fatal(err)
	}
	ps := policy.NewStore(pool)
	want := policy.Default()
	want.StoreTranscript = false
	want.RetentionDays = 30
	if err := ps.Update(ctx, a, want); err != nil {
		t.Fatal(err)
	}
	got, err := ps.Get(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}
