package worker_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/index"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/testutil"
	"sharedmemory/internal/worker"
)

func TestHandler_KBParse_ReadyChunksAndIndex(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	idx := &kbRecordingIndex{}
	store := kb.NewStore(pool, objects, rec, &index.Service{Text: idx})

	body := []byte("# Deploy\n\nUse port 15432 for postgres.\n\nKeep tokens out of logs.")
	doc, err := store.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "Ops Guide",
		MIME:  "text/markdown",
		Name:  "ops.md",
		Body:  body,
	})
	if err != nil {
		t.Fatal(err)
	}

	h := &worker.Handler{
		Pool:      pool,
		Objects:   objects,
		Extractor: worker.TextExtractor{},
		KB:        store,
	}
	if err := h.Handle(ctx, queue.Job{
		Type: queue.JobKBParse, AccountID: acct, DocumentID: doc.ID,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(ctx, acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != kb.StatusReady {
		t.Fatalf("status = %q error=%q, want ready", got.Status, got.Error)
	}

	rows, err := pool.Query(ctx,
		`SELECT id FROM kb_chunks WHERE account_id = $1 AND document_id = $2`, acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) < 1 {
		t.Fatal("expected ≥1 chunk")
	}
	chunks, err := store.GetManyChunks(ctx, acct, ids)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, c := range chunks {
		joined += c.Body
	}
	if !strings.Contains(joined, "15432") {
		t.Fatalf("chunk text missing content: %q", joined)
	}
	if len(idx.docs) < 1 {
		t.Fatal("expected IndexDoc calls")
	}
	for _, d := range idx.docs {
		if d.DocType != index.DocKBChunk {
			t.Fatalf("doc_type = %q", d.DocType)
		}
		if d.ProjectID != nil {
			t.Fatal("project_id must be nil")
		}
		if d.Kind != "Ops Guide" {
			t.Fatalf("kind = %q, want document title", d.Kind)
		}
	}
}

func TestHandler_KBParse_EmptyExtractFails(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	store := kb.NewStore(pool, objects, &queue.Recorder{}, nil)

	doc, err := store.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "Empty PDF",
		MIME:  "application/pdf",
		Name:  "empty.pdf",
		Body:  []byte("%PDF-1.4 empty"),
	})
	if err != nil {
		t.Fatal(err)
	}

	h := &worker.Handler{
		Pool:      pool,
		Objects:   objects,
		Extractor: worker.TextExtractor{}, // pdf → empty
		KB:        store,
	}
	if err := h.Handle(ctx, queue.Job{
		Type: queue.JobKBParse, AccountID: acct, DocumentID: doc.ID,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != kb.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.Error != "no text extracted" {
		t.Fatalf("error = %q", got.Error)
	}
}

type kbRecordingIndex struct {
	docs []index.Doc
}

func (r *kbRecordingIndex) Ensure(context.Context) error { return nil }

func (r *kbRecordingIndex) Index(_ context.Context, d index.Doc) error {
	r.docs = append(r.docs, d)
	return nil
}

func (r *kbRecordingIndex) Search(context.Context, index.Query) ([]index.Hit, error) {
	return nil, nil
}

func (r *kbRecordingIndex) Delete(_ context.Context, _, id uuid.UUID) error {
	out := r.docs[:0]
	for _, d := range r.docs {
		if d.ID != id {
			out = append(out, d)
		}
	}
	r.docs = out
	return nil
}

func (r *kbRecordingIndex) DeleteProject(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *kbRecordingIndex) DeleteAccount(context.Context, uuid.UUID) error           { return nil }
