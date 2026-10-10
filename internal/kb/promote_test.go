package kb_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/attachment"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/testutil"
)

func TestStore_AddFromArtifact(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	arts := artifact.NewStore(pool, objects)
	atts := &attachment.Service{Pool: pool, Objects: objects, Policies: policy.NewStore(pool), Projects: project.NewStore(pool), Queue: rec}
	store := kb.NewStore(pool, objects, rec, nil).WithSources(arts, atts)

	body := "# promoted artifact\n\n" + uuid.NewString()
	detail, err := arts.Create(ctx, acct, artifact.Input{
		Title: "Design Doc",
		Type:  "doc",
		Body:  body,
	}, artifact.Source{AgentPath: "user/manual"})
	if err != nil {
		t.Fatal(err)
	}

	doc, err := store.AddFromArtifact(ctx, acct, detail.Artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SourceType != kb.SourceArtifact {
		t.Fatalf("source_type = %q", doc.SourceType)
	}
	if doc.SourceArtifactID == nil || *doc.SourceArtifactID != detail.Artifact.ID {
		t.Fatalf("source_artifact_id = %v", doc.SourceArtifactID)
	}
	if doc.Status != kb.StatusPending {
		t.Fatalf("status = %q", doc.Status)
	}
	if doc.MIME != "text/markdown" {
		t.Fatalf("mime = %q", doc.MIME)
	}
	if doc.Title != "Design Doc" {
		t.Fatalf("title = %q", doc.Title)
	}
	if len(rec.Jobs) != 1 || rec.Jobs[0].Type != queue.JobKBParse || rec.Jobs[0].DocumentID != doc.ID {
		t.Fatalf("jobs = %+v", rec.Jobs)
	}
	key := kb.ObjectKey(acct, doc.ID)
	rc, err := objects.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != body {
		t.Fatalf("stored body mismatch")
	}

	// Cross-account isolation.
	other := testutil.NewAccount(t, pool)
	if _, err := store.AddFromArtifact(ctx, other, detail.Artifact.ID); !errors.Is(err, kb.ErrNotFound) {
		t.Fatalf("other account err = %v, want kb.ErrNotFound", err)
	}
}

func TestStore_AddFromAttachment_DerivedText(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	arts := artifact.NewStore(pool, objects)
	atts := &attachment.Service{Pool: pool, Objects: objects, Policies: policy.NewStore(pool), Projects: project.NewStore(pool), Queue: rec}
	store := kb.NewStore(pool, objects, rec, nil).WithSources(arts, atts)

	text := "derived body " + uuid.NewString()
	a, _, err := atts.Upload(ctx, acct, attachment.Upload{
		Filename: "notes.pdf",
		MIME:     "application/pdf",
		Body:     bytes.NewReader([]byte("%PDF-1.4 fake")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE attachments SET derived_text = $1 WHERE id = $2 AND account_id = $3`,
		text, a.ID, acct); err != nil {
		t.Fatal(err)
	}

	doc, err := store.AddFromAttachment(ctx, acct, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SourceType != kb.SourceAttachment {
		t.Fatalf("source_type = %q", doc.SourceType)
	}
	if doc.SourceAttachmentID == nil || *doc.SourceAttachmentID != a.ID {
		t.Fatalf("source_attachment_id = %v", doc.SourceAttachmentID)
	}
	if doc.MIME != "text/plain" {
		t.Fatalf("mime = %q, want text/plain from derived_text", doc.MIME)
	}
	key := kb.ObjectKey(acct, doc.ID)
	rc, err := objects.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != text {
		t.Fatalf("body = %q, want derived text", got)
	}

	// Cross-account isolation.
	other := testutil.NewAccount(t, pool)
	if _, err := store.AddFromAttachment(ctx, other, a.ID); !errors.Is(err, kb.ErrNotFound) {
		t.Fatalf("other account err = %v, want kb.ErrNotFound", err)
	}
}

func TestStore_AddFromAttachment_OriginalBytes(t *testing.T) {
	ctx := context.Background()
	pool := testutil.TestPool(t)
	acct := testutil.NewAccount(t, pool)
	objects := objectstore.NewMemory()
	rec := &queue.Recorder{}
	arts := artifact.NewStore(pool, objects)
	atts := &attachment.Service{Pool: pool, Objects: objects, Policies: policy.NewStore(pool), Projects: project.NewStore(pool), Queue: rec}
	store := kb.NewStore(pool, objects, rec, nil).WithSources(arts, atts)

	raw := []byte("# markdown attachment\n\n" + uuid.NewString())
	a, _, err := atts.Upload(ctx, acct, attachment.Upload{
		Filename: "guide.md",
		MIME:     "text/markdown",
		Body:     bytes.NewReader(raw),
	})
	if err != nil {
		t.Fatal(err)
	}

	doc, err := store.AddFromAttachment(ctx, acct, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.MIME != "text/markdown" {
		t.Fatalf("mime = %q", doc.MIME)
	}
	key := kb.ObjectKey(acct, doc.ID)
	rc, err := objects.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, raw) {
		t.Fatalf("body mismatch")
	}
}
