package kb_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

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
