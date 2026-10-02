//go:build integration

package recall_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/index"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/testutil"
)

func testIndex(t *testing.T) *index.Service {
	t.Helper()
	emb := semantic.HashEmbedder{}
	idx := &index.Service{
		Text:     index.NewOpenSearch("http://127.0.0.1:19200", "sm_memories_test"),
		Vector:   index.NewQdrant("http://127.0.0.1:16333", "sm_memories_test", emb.Dim()),
		Embedder: emb,
	}
	if err := idx.Ensure(context.Background()); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	return idx
}

func newService(pool *pgxpool.Pool, idx *index.Service) (*recall.Service, *memory.Store) {
	mem := memory.NewStore(pool, idx)
	return &recall.Service{Pool: pool, Index: idx, Memories: mem, Policies: policy.NewStore(pool)}, mem
}

func TestRecall_IsolatedByAccountAndProject(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	idx := testIndex(t)
	svc, mem := newService(pool, idx)

	a := testutil.NewAccount(t, pool)
	b := testutil.NewAccount(t, pool)
	t.Cleanup(func() {
		_ = idx.DeleteAccount(context.Background(), a)
		_ = idx.DeleteAccount(context.Background(), b)
	})
	projects := project.NewStore(pool)
	pa, _ := projects.Create(ctx, a, "alpha")
	pa2, _ := projects.Create(ctx, a, "beta")

	marker := "zebra" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	mustAdd(t, mem, a, memory.Memory{ProjectID: &pa.ID, Scope: semantic.ScopeProject, Kind: semantic.KindDecision,
		Body: "我们决定用 Qdrant 做向量检索 " + marker, SourceAgentPath: "cursor"})
	mustAdd(t, mem, a, memory.Memory{Scope: semantic.ScopeGlobal, Kind: semantic.KindPreference,
		Body: "我偏好简洁的回答 " + marker, SourceAgentPath: "codex"})
	mustAdd(t, mem, a, memory.Memory{ProjectID: &pa2.ID, Scope: semantic.ScopeProject, Kind: semantic.KindFact,
		Body: "beta 项目使用 MySQL " + marker, SourceAgentPath: "hermes"})

	res, err := svc.Recall(ctx, a, recall.Request{Query: marker + " Qdrant 向量", ProjectID: &pa.ID, TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("A in alpha: hits = %d (%+v), warnings %v", len(res.Hits), res.Hits, res.Warnings)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("hits must come from OpenSearch/Qdrant, not a fallback: %v", res.Warnings)
	}
	for _, h := range res.Hits {
		if strings.Contains(h.Body, "MySQL") {
			t.Fatal("project beta memory leaked into alpha recall")
		}
	}
	if !strings.Contains(res.Context, "共享记忆") || !strings.Contains(res.Context, "cursor") {
		t.Fatalf("context = %q", res.Context)
	}

	global, err := svc.Recall(ctx, a, recall.Request{Query: marker, TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(global.Hits) != 1 || global.Hits[0].Kind != semantic.KindPreference {
		t.Fatalf("no project: only global memories expected, got %+v", global.Hits)
	}

	resB, err := svc.Recall(ctx, b, recall.Request{Query: marker + " Qdrant 向量 简洁", TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resB.Hits) != 0 {
		t.Fatalf("B must not see A's memories, got %+v", resB.Hits)
	}
	raw, _ := json.Marshal(resB)
	if strings.Contains(string(raw), marker) {
		t.Fatal("A's marker appears in B's recall response")
	}
}

func TestRecall_InvalidatedMemoryDisappears(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	idx := testIndex(t)
	svc, mem := newService(pool, idx)
	a := testutil.NewAccount(t, pool)
	t.Cleanup(func() { _ = idx.DeleteAccount(context.Background(), a) })

	marker := "kiwi" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	m := mustAdd(t, mem, a, memory.Memory{Scope: semantic.ScopeGlobal, Kind: semantic.KindFact, Body: "旧事实 " + marker})
	fixed, err := mem.Supersede(ctx, a, m.ID, "新事实 "+marker)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Recall(ctx, a, recall.Request{Query: marker, TopK: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].ID != fixed.ID {
		t.Fatalf("only the correction must be recalled: %+v", res.Hits)
	}
	if err := mem.Invalidate(ctx, a, fixed.ID); err != nil {
		t.Fatal(err)
	}
	res, _ = svc.Recall(ctx, a, recall.Request{Query: marker, TopK: 10})
	if len(res.Hits) != 0 {
		t.Fatalf("invalidated memory recalled: %+v", res.Hits)
	}
}

func TestRecall_FallsBackToPostgresWhenEnginesDown(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	emb := semantic.HashEmbedder{}
	down := &index.Service{
		Text:     index.NewOpenSearch("http://127.0.0.1:1", ""),
		Vector:   index.NewQdrant("http://127.0.0.1:1", "", emb.Dim()),
		Embedder: emb,
	}
	svc, _ := newService(pool, down)
	plain := memory.NewStore(pool, nil)
	a := testutil.NewAccount(t, pool)
	marker := "mango" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	mustAdd(t, plain, a, memory.Memory{Scope: semantic.ScopeGlobal, Kind: semantic.KindFact, Body: "降级检索 " + marker})

	res, err := svc.Recall(ctx, a, recall.Request{Query: marker, TopK: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("fallback hits = %+v, warnings %v", res.Hits, res.Warnings)
	}
	if !hasWarning(res.Warnings, recall.WarnIndexDegraded) || !hasWarning(res.Warnings, recall.WarnPGFallback) {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func mustAdd(t *testing.T, s *memory.Store, acct uuid.UUID, m memory.Memory) memory.Memory {
	t.Helper()
	out, _, err := s.Add(context.Background(), acct, m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func hasWarning(list []string, w string) bool {
	for _, s := range list {
		if s == w {
			return true
		}
	}
	return false
}
