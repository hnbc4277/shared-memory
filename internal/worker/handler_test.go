//go:build integration

package worker_test

import (
	"context"
	"strings"
	"testing"

	"sharedmemory/internal/attachment"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/session"
	"sharedmemory/internal/testutil"
	"sharedmemory/internal/worker"
)

func TestHandler_DerivesSummariesPreferencesAndProfile(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	mem := memory.NewStore(pool, nil)
	sessions := &session.Service{Pool: pool, Objects: objects, Queue: rec, Policies: policy.NewStore(pool),
		Projects: project.NewStore(pool)}
	h := &worker.Handler{Pool: pool, Objects: objects, Memories: mem, Deriver: semantic.Heuristic{}, Extractor: worker.TextExtractor{}}

	acct := testutil.NewAccount(t, pool)
	p, err := project.NewStore(pool).Create(ctx, acct, "shared-memory")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sessions.Ingest(ctx, acct, session.IngestMessageRequest{
		ProjectID: p.ID.String(), Agent: "cursor", Skills: testutil.Strs(), Plugins: testutil.Strs(), Tools: testutil.Strs(),
		Role: "user", Content: "我偏好使用 Go 写后端。我们决定采用方案二。项目使用 Docker Compose 隔离。",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range rec.Jobs {
		if err := h.Handle(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	// Re-processing the same job (at-least-once delivery) must not duplicate memories.
	if err := h.Handle(ctx, rec.Jobs[0]); err != nil {
		t.Fatal(err)
	}

	all, err := mem.List(ctx, acct, memory.Filter{ProjectID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, m := range all {
		kinds[m.Kind]++
		if m.SourceAgentPath != "cursor" {
			t.Errorf("memory %s lost provenance: %q", m.Kind, m.SourceAgentPath)
		}
	}
	for _, k := range []string{semantic.KindEpisodeSummary, semantic.KindRunningSummary, semantic.KindPreference,
		semantic.KindDecision, semantic.KindFact} {
		if kinds[k] != 1 {
			t.Errorf("kind %s count = %d, want 1 (all: %v)", k, kinds[k], kinds)
		}
	}
	prefs, _ := mem.List(ctx, acct, memory.Filter{Kind: semantic.KindPreference})
	if len(prefs) != 1 || prefs[0].Scope != semantic.ScopeGlobal || prefs[0].ProjectID != nil {
		t.Fatalf("preference must be global: %+v", prefs)
	}
	profile, _, err := mem.Profile(ctx, acct)
	if err != nil || !strings.Contains(profile, "Go") {
		t.Fatalf("profile = %q, %v", profile, err)
	}
}

func TestHandler_ExtractsAttachmentText(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	mem := memory.NewStore(pool, nil)
	atts := &attachment.Service{Pool: pool, Objects: objects, Policies: policy.NewStore(pool), Projects: project.NewStore(pool), Queue: rec}
	h := &worker.Handler{Pool: pool, Objects: objects, Memories: mem, Deriver: semantic.Heuristic{}, Extractor: worker.TextExtractor{}}

	acct := testutil.NewAccount(t, pool)
	a, _, err := atts.Upload(ctx, acct, attachment.Upload{AgentPath: "codex", Filename: "notes.md", MIME: "text/markdown",
		Body: strings.NewReader("# 部署说明\n数据库端口 15432，token: abcdef123")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Jobs) != 1 {
		t.Fatalf("jobs = %d", len(rec.Jobs))
	}
	if err := h.Handle(ctx, rec.Jobs[0]); err != nil {
		t.Fatal(err)
	}
	got, _, err := atts.Get(ctx, acct, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DerivedText == nil || !strings.Contains(*got.DerivedText, "15432") {
		t.Fatalf("derived text = %v", got.DerivedText)
	}
	if strings.Contains(*got.DerivedText, "abcdef123") {
		t.Fatal("secret leaked into derived text")
	}
	list, _ := mem.List(ctx, acct, memory.Filter{Kind: semantic.KindAttachmentText})
	if len(list) != 1 || list[0].SourceAgentPath != "codex" {
		t.Fatalf("attachment memory = %+v", list)
	}
}

func TestUpload_PolicyOffStoresMetadataOnly(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	pols := policy.NewStore(pool)
	atts := &attachment.Service{Pool: pool, Objects: objects, Policies: pols, Projects: project.NewStore(pool), Queue: rec}
	acct := testutil.NewAccount(t, pool)
	pol := policy.Default()
	pol.StoreAttachments = false
	if err := pols.Update(ctx, acct, pol); err != nil {
		t.Fatal(err)
	}
	a, warnings, err := atts.Upload(ctx, acct, attachment.Upload{Filename: "a.png", MIME: "image/png", Body: strings.NewReader("png")})
	if err != nil {
		t.Fatal(err)
	}
	if a.BinaryStored || len(warnings) == 0 || len(rec.Jobs) != 0 {
		t.Fatalf("binary must not be stored: %+v %v", a, warnings)
	}
	keys, _ := objects.ListKeys(ctx, objectstore.AccountPrefix(acct))
	if len(keys) != 0 {
		t.Fatalf("objects written: %v", keys)
	}
	if _, _, err := atts.Upload(ctx, acct, attachment.Upload{Filename: "x.exe", MIME: "application/x-msdownload", Body: strings.NewReader("MZ")}); err == nil {
		t.Fatal("executables must be rejected")
	}
}
