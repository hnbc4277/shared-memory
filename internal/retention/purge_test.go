//go:build integration

package retention_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/retention"
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
	purger   *retention.Purger
}

func newWorld(t *testing.T) world {
	pool := testutil.TestPool(t)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	cipher, _ := vault.NewCipher("0123456789abcdef0123456789abcdef")
	return world{
		pool: pool, objects: objects, queue: rec,
		sessions: &session.Service{Pool: pool, Objects: objects, Queue: rec, Policies: policy.NewStore(pool),
			Projects: project.NewStore(pool), Vault: vault.NewStore(pool, cipher), Artifacts: artifact.NewStore(pool, objects)},
		worker: &worker.Handler{Pool: pool, Objects: objects, Memories: memory.NewStore(pool, nil),
			Deriver: semantic.Heuristic{}, Extractor: worker.TextExtractor{}},
		purger: &retention.Purger{Pool: pool, Objects: objects},
	}
}

// seed writes a two-message session that yields transcripts, memories, a
// vault candidate and an artifact candidate, then ages it by daysAgo.
func (w world) seed(t *testing.T, acct uuid.UUID, daysAgo int) uuid.UUID {
	ctx := context.Background()
	var sessionID uuid.UUID
	for _, role := range []string{"user", "assistant"} {
		content := "我们决定采用 Go。登录 https://x.example.com 密码: Pa55word!"
		if role == "assistant" {
			content = "# 学习笔记\n" + strings.Repeat("内容很长。", 300)
		}
		sid := ""
		if sessionID != uuid.Nil {
			sid = sessionID.String()
		}
		res, err := w.sessions.Ingest(ctx, acct, session.IngestMessageRequest{SessionID: sid, Agent: "hermes",
			Skills: testutil.Strs(), Plugins: testutil.Strs(), Tools: testutil.Strs(), Role: role, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		sessionID = res.SessionID
	}
	for _, j := range w.queue.Jobs {
		if err := w.worker.Handle(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	w.queue.Jobs = nil
	if _, err := w.pool.Exec(ctx, `UPDATE sessions SET started_at = now() - make_interval(days => $2),
		ended_at = now() - make_interval(days => $2) WHERE id = $1`, sessionID, daysAgo); err != nil {
		t.Fatal(err)
	}
	return sessionID
}

func (w world) setRetention(t *testing.T, acct uuid.UUID, days int) {
	store := policy.NewStore(w.pool)
	p, err := store.Get(context.Background(), acct)
	if err != nil {
		t.Fatal(err)
	}
	p.RetentionDays = days
	if err := store.Update(context.Background(), acct, p); err != nil {
		t.Fatal(err)
	}
}

func (w world) countWhere(t *testing.T, q string, args ...any) int {
	var n int
	if err := w.pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (w world) transcriptKeys(t *testing.T, acct, sessionID uuid.UUID) []string {
	keys, err := w.objects.ListKeys(context.Background(), objectstore.TranscriptPrefix(acct, nil, sessionID))
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestPurge_DropsExpiredEvidenceAndKeepsMemories(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	acct := testutil.NewAccount(t, w.pool)
	w.setRetention(t, acct, 30)
	old := w.seed(t, acct, 40)
	fresh := w.seed(t, acct, 1)
	memoriesBefore := w.countWhere(t, `SELECT count(*) FROM memories WHERE source_session_id = $1`, old)
	if memoriesBefore == 0 || len(w.transcriptKeys(t, acct, old)) == 0 {
		t.Fatal("seed must produce memories and transcripts")
	}

	if _, err := w.purger.Run(ctx); err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{
		`SELECT count(*) FROM sessions WHERE id = $1`,
		`SELECT count(*) FROM messages WHERE session_id = $1`,
		`SELECT count(*) FROM events WHERE session_id = $1`,
		`SELECT count(*) FROM vault_candidates WHERE session_id = $1`,
		`SELECT count(*) FROM artifact_candidates WHERE session_id = $1`,
		`SELECT count(*) FROM memories WHERE source_session_id = $1`,
	} {
		if n := w.countWhere(t, q, old); n != 0 {
			t.Errorf("%s: %d rows left", q, n)
		}
	}
	if keys := w.transcriptKeys(t, acct, old); len(keys) != 0 {
		t.Fatalf("expired transcripts left: %v", keys)
	}
	if n := w.countWhere(t, `SELECT count(*) FROM memories WHERE account_id = $1 AND source_session_id IS NULL`, acct); n < memoriesBefore {
		t.Fatalf("derived memories must survive the purge: %d < %d", n, memoriesBefore)
	}
	if w.countWhere(t, `SELECT count(*) FROM messages WHERE session_id = $1`, fresh) != 2 || len(w.transcriptKeys(t, acct, fresh)) == 0 {
		t.Fatal("sessions inside the retention window must survive")
	}
}

func TestPurge_ZeroRetentionKeepsEverything(t *testing.T) {
	w := newWorld(t)
	acct := testutil.NewAccount(t, w.pool)
	old := w.seed(t, acct, 400)

	if _, err := w.purger.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w.countWhere(t, `SELECT count(*) FROM messages WHERE session_id = $1`, old) != 2 {
		t.Fatal("retention_days = 0 means keep forever")
	}
}

func TestPurge_RemovesExpiredArtifactCandidates(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	acct := testutil.NewAccount(t, w.pool)
	sid := w.seed(t, acct, 0)
	var key string
	if err := w.pool.QueryRow(ctx, `UPDATE artifact_candidates SET expires_at = now() - interval '1 minute'
		WHERE session_id = $1 RETURNING body_object_key`, sid).Scan(&key); err != nil {
		t.Fatal(err)
	}

	if _, err := w.purger.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if w.countWhere(t, `SELECT count(*) FROM artifact_candidates WHERE session_id = $1`, sid) != 0 {
		t.Fatal("expired artifact candidate must be removed")
	}
	if _, err := w.objects.Get(ctx, key); err == nil {
		t.Fatal("expired artifact candidate body must be removed from object storage")
	}
}
