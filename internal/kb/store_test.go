package kb_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/index"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/testutil"
)

func TestStore_AddUploadDedupDelete(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	store := kb.NewStore(pool, objects, rec, nil)

	body := []byte("# hello\n\nkb upload test " + uuid.NewString())
	doc, err := store.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "Guide",
		MIME:  "text/markdown",
		Name:  "guide.md",
		Body:  body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != kb.StatusPending {
		t.Fatalf("status = %q, want pending", doc.Status)
	}
	if doc.SourceType != kb.SourceUpload {
		t.Fatalf("source = %q", doc.SourceType)
	}
	if len(rec.Jobs) != 1 || rec.Jobs[0].Type != queue.JobKBParse {
		t.Fatalf("jobs = %+v", rec.Jobs)
	}
	if rec.Jobs[0].DocumentID != doc.ID || rec.Jobs[0].AccountID != acct {
		t.Fatalf("job ids = %+v", rec.Jobs[0])
	}
	key := kb.ObjectKey(acct, doc.ID)
	if _, err := objects.Get(ctx, key); err != nil {
		t.Fatalf("object missing: %v", err)
	}

	// Dedup only applies to ready docs.
	_, err = pool.Exec(ctx, `UPDATE kb_documents SET status = $1 WHERE id = $2 AND account_id = $3`,
		kb.StatusReady, doc.ID, acct)
	if err != nil {
		t.Fatal(err)
	}

	again, err := store.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "Other title",
		MIME:  "text/markdown",
		Name:  "guide.md",
		Body:  body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != doc.ID {
		t.Fatalf("dedup id = %s, want %s", again.ID, doc.ID)
	}
	if len(rec.Jobs) != 1 {
		t.Fatalf("dedup must not enqueue again, jobs=%d", len(rec.Jobs))
	}

	got, err := store.Get(ctx, acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Guide" {
		t.Fatalf("title = %q, want Guide (dedup keeps original)", got.Title)
	}

	list, err := store.List(ctx, acct, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range list {
		if d.ID == doc.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("list missing document")
	}

	if err := store.Delete(ctx, acct, doc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, acct, doc.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
	if _, err := objects.Get(ctx, key); err == nil {
		t.Fatal("expected object deleted")
	}
}

func TestStore_AddUpload_UnsupportedMIME(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	store := kb.NewStore(pool, objectstore.NewMemory(), &queue.Recorder{}, nil)
	_, err := store.AddUpload(ctx, acct, kb.AddUploadInput{
		MIME: "image/png",
		Name: "x.png",
		Body: []byte("x"),
	})
	if err == nil {
		t.Fatal("expected unsupported")
	}
}

func TestStore_SetStatusAndReplaceChunks(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	idx := &recordingIndex{}
	store := kb.NewStore(pool, objects, &queue.Recorder{}, &index.Service{Text: idx})

	doc, err := store.AddUpload(ctx, acct, kb.AddUploadInput{
		Title: "Guide",
		MIME:  "text/markdown",
		Name:  "guide.md",
		Body:  []byte("# hello\n\nchunk body " + uuid.NewString()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetStatus(ctx, acct, doc.ID, kb.StatusFailed, "boom"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != kb.StatusFailed || got.Error != "boom" {
		t.Fatalf("status=%q error=%q", got.Status, got.Error)
	}

	chunks := []kb.Chunk{
		{Ordinal: 0, Body: "first paragraph"},
		{Ordinal: 1, Body: "second paragraph"},
	}
	if err := store.ReplaceChunks(ctx, acct, doc.ID, chunks); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get(ctx, acct, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != kb.StatusReady || got.Error != "" {
		t.Fatalf("after replace status=%q error=%q", got.Status, got.Error)
	}
	ids := listChunkIDs(t, pool, acct, doc.ID)
	stored, err := store.GetManyChunks(ctx, acct, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("chunks = %d, want 2", len(stored))
	}
	if len(idx.docs) != 2 {
		t.Fatalf("indexed = %d, want 2", len(idx.docs))
	}
	for _, d := range idx.docs {
		if d.DocType != index.DocKBChunk {
			t.Fatalf("doc_type = %q", d.DocType)
		}
		if d.ProjectID != nil {
			t.Fatal("project_id must be nil")
		}
		if d.Kind != "Guide" {
			t.Fatalf("kind (title) = %q", d.Kind)
		}
		if d.AccountID != acct {
			t.Fatalf("account = %s", d.AccountID)
		}
	}

	if err := store.ReplaceChunks(ctx, acct, doc.ID, []kb.Chunk{{Ordinal: 0, Body: "only one"}}); err != nil {
		t.Fatal(err)
	}
	ids = listChunkIDs(t, pool, acct, doc.ID)
	stored, err = store.GetManyChunks(ctx, acct, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Body != "only one" {
		t.Fatalf("replaced chunks = %+v", stored)
	}
	if len(idx.docs) != 1 {
		t.Fatalf("after replace indexed = %d, want 1", len(idx.docs))
	}
}

func listChunkIDs(t *testing.T, pool *pgxpool.Pool, acct, docID uuid.UUID) []uuid.UUID {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id FROM kb_chunks WHERE account_id = $1 AND document_id = $2 ORDER BY ordinal`,
		acct, docID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// recordingIndex is a TextIndex that records Index/Delete for assertions.
type recordingIndex struct {
	docs []index.Doc
}

func (r *recordingIndex) Ensure(context.Context) error { return nil }

func (r *recordingIndex) Index(_ context.Context, d index.Doc) error {
	r.docs = append(r.docs, d)
	return nil
}

func (r *recordingIndex) Search(context.Context, index.Query) ([]index.Hit, error) {
	return nil, nil
}

func (r *recordingIndex) Delete(_ context.Context, _, id uuid.UUID) error {
	out := r.docs[:0]
	for _, d := range r.docs {
		if d.ID != id {
			out = append(out, d)
		}
	}
	r.docs = out
	return nil
}

func (r *recordingIndex) DeleteProject(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (r *recordingIndex) DeleteAccount(context.Context, uuid.UUID) error           { return nil }
