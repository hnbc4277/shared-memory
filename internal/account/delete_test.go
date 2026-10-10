//go:build integration

package account_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/account"
	"sharedmemory/internal/artifact"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/session"
	"sharedmemory/internal/testutil"
	"sharedmemory/internal/vault"
	"sharedmemory/internal/worker"
)

type world struct {
	pool     *pgxpool.Pool
	objects  *objectstore.Memory
	sessions *session.Service
	worker   *worker.Handler
	queue    *queue.Recorder
	deleter  *account.Deleter
}

func newWorld(t *testing.T) world {
	pool := testutil.TestPool(t)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	cipher, _ := vault.NewCipher("0123456789abcdef0123456789abcdef")
	mem := memory.NewStore(pool, nil)
	return world{
		pool: pool, objects: objects, queue: rec,
		sessions: &session.Service{Pool: pool, Objects: objects, Queue: rec, Policies: policy.NewStore(pool),
			Projects: project.NewStore(pool), Vault: vault.NewStore(pool, cipher), Artifacts: artifact.NewStore(pool, objects)},
		worker:  &worker.Handler{Pool: pool, Objects: objects, Memories: mem, Deriver: semantic.Heuristic{}, Extractor: worker.TextExtractor{}},
		deleter: &account.Deleter{Pool: pool, Objects: objects},
	}
}

// seed writes a message that produces every derived row type: transcript,
// memories, a vault candidate and an artifact candidate.
func (w world) seed(t *testing.T, acct uuid.UUID, projectID string) {
	ctx := context.Background()
	sessionID := ""
	for _, role := range []string{"user", "assistant"} {
		content := "我们决定采用 Go。登录 https://x.example.com 密码: Pa55word!"
		if role == "assistant" {
			content = "# 学习笔记\n" + strings.Repeat("内容很长。", 300)
		}
		res, err := w.sessions.Ingest(ctx, acct, session.IngestMessageRequest{SessionID: sessionID, ProjectID: projectID,
			Agent: "hermes", Skills: testutil.Strs(), Plugins: testutil.Strs(), Tools: testutil.Strs(), Role: role, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		sessionID = res.SessionID.String()
	}
	for _, j := range w.queue.Jobs {
		if err := w.worker.Handle(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	w.queue.Jobs = nil
}

func (w world) count(t *testing.T, table string, acct uuid.UUID) int {
	var n int
	col := "account_id"
	if table == "accounts" {
		col = "id"
	}
	if err := w.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table+" WHERE "+col+" = $1", acct).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeleteProject_RemovesOnlyThatProject(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	acct := testutil.NewAccount(t, w.pool)
	p, err := project.NewStore(w.pool).Create(ctx, acct, "doomed")
	if err != nil {
		t.Fatal(err)
	}
	w.seed(t, acct, p.ID.String())
	w.seed(t, acct, "")

	if err := w.deleter.DeleteProject(ctx, acct, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = w.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE project_id = $1`, p.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("project sessions left: %d", n)
	}
	if w.count(t, "sessions", acct) != 1 {
		t.Fatal("global session must survive a project delete")
	}
	keys, _ := w.objects.ListKeys(ctx, objectstore.ProjectPrefix(acct, p.ID))
	if len(keys) != 0 {
		t.Fatalf("project objects left: %v", keys)
	}
	if left, _ := w.objects.ListKeys(ctx, objectstore.AccountPrefix(acct)); len(left) == 0 {
		t.Fatal("global objects must survive")
	}
	if err := w.deleter.DeleteProject(ctx, acct, p.ID); err != account.ErrNotFound {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestDeleteProject_KnowledgeBaseSurvives(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	acct := testutil.NewAccount(t, w.pool)
	p, err := project.NewStore(w.pool).Create(ctx, acct, "doomed")
	if err != nil {
		t.Fatal(err)
	}
	w.seed(t, acct, p.ID.String())
	kbStore := kb.NewStore(w.pool, w.objects, w.queue, nil)
	if _, err := kbStore.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "global kb", MIME: "text/plain", Name: "note.txt", Body: []byte("account-wide reference"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.deleter.DeleteProject(ctx, acct, p.ID); err != nil {
		t.Fatal(err)
	}
	if n := w.count(t, "kb_documents", acct); n != 1 {
		t.Fatalf("kb_documents = %d, want 1 after project delete", n)
	}
}

func TestDeleteAccount_RemovesEverything(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	acct := testutil.NewAccount(t, w.pool)
	other := testutil.NewAccount(t, w.pool)
	p, _ := project.NewStore(w.pool).Create(ctx, acct, "p")
	w.seed(t, acct, p.ID.String())
	w.seed(t, acct, "")
	w.seed(t, other, "")
	kbStore := kb.NewStore(w.pool, w.objects, w.queue, nil)
	if _, err := kbStore.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "kb", MIME: "text/markdown", Name: "doc.md", Body: []byte("# Title\nbody"),
	}); err != nil {
		t.Fatal(err)
	}

	if err := w.deleter.DeleteAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sessions", "messages", "events", "memories", "kb_documents", "kb_chunks",
		"vault_candidates", "artifact_candidates", "projects", "api_keys", "accounts"} {
		if n := w.count(t, table, acct); n != 0 {
			t.Errorf("%s: %d rows left", table, n)
		}
	}
	if keys, _ := w.objects.ListKeys(ctx, objectstore.AccountPrefix(acct)); len(keys) != 0 {
		t.Fatalf("objects left: %v", keys)
	}
	if w.count(t, "sessions", other) == 0 {
		t.Fatal("other account's data must be untouched")
	}
}
