//go:build integration

package recall_test

import (
	"context"
	"testing"

	"sharedmemory/internal/agents"
	"sharedmemory/internal/auth"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/testutil"
)

func TestRecall_FiltersHitsByConnection(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	authStore := auth.NewStore(pool)

	agentStore := agents.NewStore(pool, authStore)
	c1, err := agentStore.CreateRegistered(ctx, acct, agents.CreateInput{Agent: "cursor", Label: "A"})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := agentStore.CreateRegistered(ctx, acct, agents.CreateInput{Agent: "cursor", Label: "B"})
	if err != nil {
		t.Fatal(err)
	}

	mem := memory.NewStore(pool, nil)
	connA := c1.Connection.ID
	connB := c2.Connection.ID
	if _, _, err := mem.Add(ctx, acct, memory.Memory{
		Scope: semantic.ScopeGlobal, Kind: semantic.KindFact, Body: "connA-only-marker-alpha",
		SourceConnectionID: &connA,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mem.Add(ctx, acct, memory.Memory{
		Scope: semantic.ScopeGlobal, Kind: semantic.KindFact, Body: "connB-only-marker-beta",
		SourceConnectionID: &connB,
	}); err != nil {
		t.Fatal(err)
	}

	svc := &recall.Service{Pool: pool, Memories: mem, Policies: policy.NewStore(pool)}
	res, err := svc.Recall(ctx, acct, recall.Request{
		Query:        "connA-only-marker-alpha",
		ConnectionID: connA.String(),
		TopK:         10,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.Body == "connB-only-marker-beta" {
			t.Fatalf("other connection memory leaked into filtered recall: %+v", h)
		}
	}

	res2, err := svc.Recall(ctx, acct, recall.Request{
		Query:        "connB-only-marker-beta",
		ConnectionID: connB.String(),
		TopK:         10,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res2.Hits {
		if h.Body == "connA-only-marker-alpha" {
			t.Fatalf("other connection memory leaked: %+v", h)
		}
	}
}
