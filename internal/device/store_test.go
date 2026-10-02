//go:build integration

package device_test

import (
	"context"
	"errors"
	"testing"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/device"
	"sharedmemory/internal/testutil"
)

func TestDeviceCode_ApprovePollRevoke(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	authStore := auth.NewStore(pool)
	store := device.NewStore(pool, authStore)
	acct := testutil.NewAccount(t, pool)

	code, err := store.CreateCode(ctx, "laptop", "cursor", "http://127.0.0.1:5173")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Poll(ctx, code.DeviceCode); !errors.Is(err, device.ErrPending) {
		t.Fatalf("before approve: %v", err)
	}
	if err := store.Approve(ctx, acct, code.UserCode); err != nil {
		t.Fatal(err)
	}
	tok, err := store.Poll(ctx, code.DeviceCode)
	if err != nil {
		t.Fatal(err)
	}
	if tok.APIKey == "" || tok.AccountID != acct {
		t.Fatalf("token = %+v", tok)
	}
	got, err := authStore.AuthenticateAPIKey(ctx, tok.APIKey)
	if err != nil || got != acct {
		t.Fatalf("key auth: %v %v", got, err)
	}
	if _, err := store.Poll(ctx, code.DeviceCode); !errors.Is(err, device.ErrRedeemed) {
		t.Fatalf("second poll: %v", err)
	}
	list, err := store.List(ctx, acct)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := store.HeartbeatDevice(ctx, acct, list[0].ID, "0.1.0", 3); err != nil {
		t.Fatal(err)
	}
	list, _ = store.List(ctx, acct)
	if list[0].SpoolPending != 3 || list[0].DaemonVersion != "0.1.0" {
		t.Fatalf("heartbeat not applied: %+v", list[0])
	}
	if err := store.Revoke(ctx, acct, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := authStore.AuthenticateAPIKey(ctx, tok.APIKey); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("revoked key still works: %v", err)
	}
}

func TestNormalizeUserCode(t *testing.T) {
	if got := device.NormalizeUserCode("ab-cd-ef-gh"); got != "ABCDEFGH" {
		t.Fatalf("got %q", got)
	}
}
