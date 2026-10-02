//go:build integration

package objectstore_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"sharedmemory/internal/objectstore"
)

func TestMinIO_PutGetDeletePrefix(t *testing.T) {
	ctx := context.Background()
	m, err := objectstore.NewMinIO(ctx, "127.0.0.1:19000", "smminio", "smminio_secret", "shared-memory-test", false)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "it-" + uuid.NewString() + "/"
	key := prefix + "a.txt"
	body := []byte("hello minio")
	if err := m.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := m.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
	if u, err := m.PresignGet(ctx, key, time.Minute); err != nil || u == "" {
		t.Fatalf("presign: %v %q", err, u)
	}
	if err := m.DeletePrefix(ctx, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, key); !errors.Is(err, objectstore.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}
