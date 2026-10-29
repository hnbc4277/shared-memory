//go:build integration

package recall_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/agents"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/testutil"
)

func TestReceipt_PersistsConnectionID(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	authStore := auth.NewStore(pool)
	agentStore := agents.NewStore(pool, authStore)
	created, err := agentStore.CreateRegistered(ctx, acct, agents.CreateInput{Agent: "cursor", Label: "Receipt conn"})
	if err != nil {
		t.Fatal(err)
	}
	cid := created.Connection.ID.String()

	svc := &recall.Service{Pool: pool, Memories: memory.NewStore(pool, nil), Policies: policy.NewStore(pool)}
	res, err := svc.Recall(ctx, acct, recall.Request{
		Query:        "receipt connection " + uuid.NewString(),
		ConnectionID: cid,
		Source:       "http",
		TopK:         3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReceiptID == uuid.Nil {
		t.Fatal("expected receipt id")
	}
	var stored *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT connection_id FROM recall_receipts WHERE id = $1`, res.ReceiptID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == nil || *stored != created.Connection.ID {
		t.Fatalf("stored connection_id = %v want %s", stored, created.Connection.ID)
	}
	list, err := svc.ListReceipts(ctx, acct, recall.ReceiptFilter{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range list {
		if r.ID == res.ReceiptID {
			found = true
			if r.ConnectionID == nil || *r.ConnectionID != created.Connection.ID {
				t.Fatalf("list receipt connection_id = %v", r.ConnectionID)
			}
		}
	}
	if !found {
		t.Fatal("receipt not in list")
	}
	filtered, err := svc.ListReceipts(ctx, acct, recall.ReceiptFilter{
		ConnectionID: &created.Connection.ID,
		Limit:        10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) < 1 {
		t.Fatal("expected receipts for connection filter")
	}
	for _, r := range filtered {
		if r.ConnectionID == nil || *r.ConnectionID != created.Connection.ID {
			t.Fatalf("filtered receipt connection_id = %v", r.ConnectionID)
		}
	}
}
