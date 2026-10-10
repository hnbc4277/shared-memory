package recall_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/index"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/testutil"
)

// memTextIndex is an in-memory TextIndex used to exercise KB partition without OpenSearch.
type memTextIndex struct {
	docs []index.Doc
}

func (m *memTextIndex) Ensure(context.Context) error { return nil }

func (m *memTextIndex) Index(_ context.Context, d index.Doc) error {
	m.docs = append(m.docs, d)
	return nil
}

func (m *memTextIndex) Search(_ context.Context, q index.Query) ([]index.Hit, error) {
	var out []index.Hit
	for _, d := range m.docs {
		if d.AccountID != q.AccountID {
			continue
		}
		if q.ProjectID != nil && d.ProjectID != nil && *d.ProjectID != *q.ProjectID {
			continue
		}
		matched := strings.Contains(d.Body, q.Text)
		if !matched {
			for _, tok := range strings.Fields(q.Text) {
				if tok != "" && strings.Contains(d.Body, tok) {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		out = append(out, index.Hit{ID: d.ID, DocType: d.DocType, Score: 1})
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func (m *memTextIndex) Delete(_ context.Context, _, id uuid.UUID) error {
	kept := m.docs[:0]
	for _, d := range m.docs {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	m.docs = kept
	return nil
}

func (m *memTextIndex) DeleteProject(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (m *memTextIndex) DeleteAccount(_ context.Context, accountID uuid.UUID) error {
	kept := m.docs[:0]
	for _, d := range m.docs {
		if d.AccountID != accountID {
			kept = append(kept, d)
		}
	}
	m.docs = kept
	return nil
}

type kbFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	svc     *recall.Service
	mem     *memory.Store
	kb      *kb.Store
	pol     *policy.Store
	acct    uuid.UUID
	textIdx *memTextIndex
}

func setupKBRecall(t *testing.T) *kbFixture {
	t.Helper()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	text := &memTextIndex{}
	idx := &index.Service{Text: text}
	mem := memory.NewStore(pool, idx)
	kbStore := kb.NewStore(pool, objectstore.NewMemory(), &queue.Recorder{}, idx)
	pol := policy.NewStore(pool)
	return &kbFixture{
		ctx: context.Background(), pool: pool,
		svc: &recall.Service{Pool: pool, Index: idx, Memories: mem, Policies: pol, KB: kbStore},
		mem: mem, kb: kbStore, pol: pol, acct: acct, textIdx: text,
	}
}

func (f *kbFixture) seedReadyChunk(t *testing.T, title, body string) (docID, chunkID uuid.UUID) {
	t.Helper()
	doc, err := f.kb.AddUpload(f.ctx, f.acct, kb.AddUploadInput{
		Title: title, MIME: "text/markdown", Name: "g.md", Body: []byte(body),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.kb.ReplaceChunks(f.ctx, f.acct, doc.ID, []kb.Chunk{{Ordinal: 0, Body: body}}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(f.ctx,
		`SELECT id FROM kb_chunks WHERE account_id = $1 AND document_id = $2 ORDER BY ordinal`,
		f.acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no chunk")
	}
	if err := rows.Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	return doc.ID, chunkID
}

func TestRecall_KBHits_ReadyChunk(t *testing.T) {
	f := setupKBRecall(t)
	marker := "kbalpha" + uuid.NewString()[:8]
	docID, chunkID := f.seedReadyChunk(t, "Ops Guide", "Deploy with "+marker+" on port 15432")

	res, err := f.svc.Recall(f.ctx, f.acct, recall.Request{Query: marker, TopK: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.KBHits) != 1 {
		t.Fatalf("kb_hits = %+v, want 1", res.KBHits)
	}
	h := res.KBHits[0]
	if h.ID != chunkID || h.DocumentID != docID {
		t.Fatalf("ids = %+v, want chunk=%s doc=%s", h, chunkID, docID)
	}
	if h.Title != "Ops Guide" || !strings.Contains(h.Body, marker) {
		t.Fatalf("hit = %+v", h)
	}
	if !strings.Contains(res.Context, "## Knowledge Base") || !strings.Contains(res.Context, "Ops Guide") {
		t.Fatalf("context missing KB section: %q", res.Context)
	}
}

func TestRecall_KBHits_ProjectIDStillReturnsGlobalKB(t *testing.T) {
	f := setupKBRecall(t)
	marker := "kbproj" + uuid.NewString()[:8]
	_, chunkID := f.seedReadyChunk(t, "Global Doc", "KB fact "+marker)
	p, err := project.NewStore(f.pool).Create(f.ctx, f.acct, "alpha")
	if err != nil {
		t.Fatal(err)
	}

	res, err := f.svc.Recall(f.ctx, f.acct, recall.Request{Query: marker, ProjectID: &p.ID, TopK: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.KBHits) != 1 || res.KBHits[0].ID != chunkID {
		t.Fatalf("project-scoped recall must still return global KB, got %+v", res.KBHits)
	}
}

func TestRecall_KBHits_InjectDisabled(t *testing.T) {
	f := setupKBRecall(t)
	marker := "kbinj" + uuid.NewString()[:8]
	f.seedReadyChunk(t, "Doc", "body "+marker)
	pol := policy.Default()
	pol.InjectEnabled = false
	if err := f.pol.Update(f.ctx, f.acct, pol); err != nil {
		t.Fatal(err)
	}

	res, err := f.svc.Recall(f.ctx, f.acct, recall.Request{Query: marker, TopK: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.KBHits) != 0 {
		t.Fatalf("inject_enabled=false must empty kb_hits, got %+v", res.KBHits)
	}
	if res.Context != "" {
		t.Fatalf("context must be empty when inject disabled, got %q", res.Context)
	}
}

func TestRecall_KBHits_KBInjectDisabledKeepsMemory(t *testing.T) {
	f := setupKBRecall(t)
	marker := "kbmem" + uuid.NewString()[:8]
	f.seedReadyChunk(t, "Doc", "kb side "+marker)
	if _, _, err := f.mem.Add(f.ctx, f.acct, memory.Memory{
		Scope: semantic.ScopeGlobal, Kind: semantic.KindFact,
		Body: "memory side " + marker, SourceAgentPath: "cursor",
	}); err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	pol.KBInjectEnabled = false
	if err := f.pol.Update(f.ctx, f.acct, pol); err != nil {
		t.Fatal(err)
	}

	res, err := f.svc.Recall(f.ctx, f.acct, recall.Request{Query: marker, TopK: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.KBHits) != 0 {
		t.Fatalf("kb_inject_enabled=false must empty kb_hits, got %+v", res.KBHits)
	}
	if len(res.Hits) != 1 || !strings.Contains(res.Hits[0].Body, "memory side") {
		t.Fatalf("memory hits must remain: %+v", res.Hits)
	}
}

func TestRecall_Receipt_KBHitCount(t *testing.T) {
	f := setupKBRecall(t)
	marker := "kbrcpt" + uuid.NewString()[:8]
	f.seedReadyChunk(t, "Doc", "receipt "+marker)

	res, err := f.svc.Recall(f.ctx, f.acct, recall.Request{
		Query: marker, TopK: 8, Source: "http", ExternalSessionID: "ext-" + marker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReceiptID == uuid.Nil {
		t.Fatal("expected receipt")
	}
	list, err := f.svc.ListReceipts(f.ctx, f.acct, recall.ReceiptFilter{
		ExternalSessionID: "ext-" + marker, Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Fatal("expected receipt row")
	}
	var found bool
	for _, r := range list {
		if r.ID == res.ReceiptID {
			found = true
			if r.KBHitCount != len(res.KBHits) || r.KBHitCount != 1 {
				t.Fatalf("kb_hit_count = %d, kb_hits=%d", r.KBHitCount, len(res.KBHits))
			}
			if r.HitCount != len(res.Hits) {
				t.Fatalf("hit_count = %d, hits=%d", r.HitCount, len(res.Hits))
			}
		}
	}
	if !found {
		t.Fatal("receipt not listed")
	}
}
