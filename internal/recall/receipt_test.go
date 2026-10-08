//go:build integration

package recall_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/testutil"
)

func TestRecall_WritesReceipt(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	svc := &recall.Service{Pool: pool, Memories: memory.NewStore(pool, nil), Policies: policy.NewStore(pool)}
	a := testutil.NewAccount(t, pool)
	b := testutil.NewAccount(t, pool)

	ext := "ext-" + uuid.NewString()
	res, err := svc.Recall(ctx, a, recall.Request{
		Query:             "receipt probe " + uuid.NewString(),
		Source:            "http",
		ExternalSessionID: ext,
		Agent:             "cursor",
		AgentPath:         "cursor",
		TopK:              5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReceiptID == uuid.Nil {
		t.Fatal("expected ReceiptID to be set")
	}

	list, err := svc.ListReceipts(ctx, a, recall.ReceiptFilter{ExternalSessionID: ext, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Fatal("expected at least one receipt")
	}
	found := false
	for _, r := range list {
		if r.ID == res.ReceiptID {
			found = true
			if r.Source != "http" {
				t.Fatalf("source = %q", r.Source)
			}
			if r.Agent != "cursor" || r.AgentPath != "cursor" {
				t.Fatalf("agent fields = %+v", r)
			}
			if r.ExternalSessionID != ext {
				t.Fatalf("external_session_id = %q", r.ExternalSessionID)
			}
			if !strings.HasPrefix(r.QueryPreview, "receipt probe") {
				t.Fatalf("query_preview = %q", r.QueryPreview)
			}
			break
		}
	}
	if !found {
		t.Fatalf("receipt %s not in ListReceipts", res.ReceiptID)
	}

	other, err := svc.ListReceipts(ctx, b, recall.ReceiptFilter{ExternalSessionID: ext, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("account B must not see A's receipts: %+v", other)
	}
}

func TestRecall_ContinuesWhenReceiptInsertFails(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	svc := &recall.Service{Pool: pool, Memories: memory.NewStore(pool, nil), Policies: policy.NewStore(pool)}
	a := testutil.NewAccount(t, pool)
	badSession := uuid.New() // not in sessions → FK failure on insert
	res, err := svc.Recall(ctx, a, recall.Request{
		Query:     "fk fail " + uuid.NewString(),
		SessionID: &badSession,
		Source:    "http",
		TopK:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReceiptID != uuid.Nil {
		t.Fatalf("expected nil ReceiptID after insert failure, got %s", res.ReceiptID)
	}
	// recall itself still returns a normal result shape
	if res.Hits == nil {
		t.Fatal("hits must be non-nil")
	}
}
