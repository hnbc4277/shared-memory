//go:build integration

package recall_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/testutil"
)

func TestPurgeReceipts_DeletesOnlyOld(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	svc := &recall.Service{Pool: pool, Memories: memory.NewStore(pool, nil), Policies: policy.NewStore(pool)}
	a := testutil.NewAccount(t, pool)

	res, err := svc.Recall(ctx, a, recall.Request{
		Query:             "purge probe " + uuid.NewString(),
		Source:            "http",
		ExternalSessionID: "purge-" + uuid.NewString(),
		TopK:              3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReceiptID == uuid.Nil {
		t.Fatal("expected receipt")
	}

	// Age the row beyond retention.
	_, err = pool.Exec(ctx, `UPDATE recall_receipts SET queried_at = $2 WHERE id = $1`,
		res.ReceiptID, time.Now().UTC().AddDate(0, 0, -40))
	if err != nil {
		t.Fatal(err)
	}

	// Fresh receipt should survive.
	fresh, err := svc.Recall(ctx, a, recall.Request{
		Query:  "fresh " + uuid.NewString(),
		Source: "mcp",
		TopK:   3,
	})
	if err != nil {
		t.Fatal(err)
	}

	n, err := recall.PurgeReceipts(ctx, pool, 30)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected to delete at least 1, got %d", n)
	}

	var oldExists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM recall_receipts WHERE id = $1)`, res.ReceiptID).Scan(&oldExists)
	if oldExists {
		t.Fatal("old receipt should be purged")
	}
	var freshExists bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM recall_receipts WHERE id = $1)`, fresh.ReceiptID).Scan(&freshExists)
	if !freshExists {
		t.Fatal("fresh receipt must remain")
	}
}

func TestCheckReceipt_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	svc := &recall.Service{Pool: pool, Memories: memory.NewStore(pool, nil), Policies: policy.NewStore(pool)}
	a := testutil.NewAccount(t, pool)
	ext := "chk-" + uuid.NewString()

	need, err := svc.CheckReceipt(ctx, a, ext, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if need.OK || !need.NeedRecall {
		t.Fatalf("before recall: %+v", need)
	}

	if _, err := svc.Recall(ctx, a, recall.Request{
		Query: "check " + uuid.NewString(), Source: "mcp", ExternalSessionID: ext, TopK: 3,
	}); err != nil {
		t.Fatal(err)
	}
	ok, err := svc.CheckReceipt(ctx, a, ext, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if !ok.OK || ok.NeedRecall || ok.Latest == nil {
		t.Fatalf("after recall: %+v", ok)
	}
}
