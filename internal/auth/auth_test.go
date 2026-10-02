//go:build integration

package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/testutil"
)

func TestAPIKey_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	store := auth.NewStore(pool)

	accountID, err := store.CreateAccount(ctx, "user-"+uuid.NewString()+"@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAPIKey(ctx, accountID, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.AuthenticateAPIKey(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if got != accountID {
		t.Fatalf("account mismatch: got %s want %s", got, accountID)
	}
	_, err = store.AuthenticateAPIKey(ctx, "sm_deadbeef")
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestRegister_ReturnsAPIKey(t *testing.T) {
	ctx := context.Background()
	store := auth.NewStore(testutil.TestPool(t))
	id, key, err := store.Register(ctx, "reg-"+uuid.NewString()+"@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() == "" || key == "" || key[:3] != "sm_" {
		t.Fatalf("unexpected register result id=%s key=%q", id, key)
	}
	got, err := store.AuthenticateAPIKey(ctx, key)
	if err != nil || got != id {
		t.Fatalf("auth after register failed: %v got=%s", err, got)
	}
}
