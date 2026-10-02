package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestSpoolEnqueueAndCount(t *testing.T) {
	dir := t.TempDir()
	s := NewSpool(dir)
	if n, err := s.Pending(); err != nil || n != 0 {
		t.Fatalf("pending=%d err=%v", n, err)
	}
	if err := s.Enqueue("ingest", []byte(`{"role":"user"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue("recall", []byte(`{"query":"x"}`)); err != nil {
		t.Fatal(err)
	}
	n, err := s.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("pending=%d", n)
	}
}

func TestSpoolReplaySuccessRemoves(t *testing.T) {
	dir := t.TempDir()
	s := NewSpool(dir)
	_ = s.Enqueue("ingest", []byte(`{"a":1}`))
	_ = s.Enqueue("ingest", []byte(`{"a":2}`))

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth=%q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/ingest" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	n, err := s.Replay(srv.Client(), srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || hits.Load() != 2 {
		t.Fatalf("replayed=%d hits=%d", n, hits.Load())
	}
	if pending, _ := s.Pending(); pending != 0 {
		t.Fatalf("pending left=%d", pending)
	}
}

func TestSpoolReplayKeepsOnFailure(t *testing.T) {
	dir := t.TempDir()
	s := NewSpool(dir)
	_ = s.Enqueue("ingest", []byte(`{"a":1}`))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, _ = s.Replay(srv.Client(), srv.URL, "tok")
	if pending, _ := s.Pending(); pending != 1 {
		t.Fatalf("pending=%d", pending)
	}
}

func TestSpoolFilesAreJSONL(t *testing.T) {
	dir := t.TempDir()
	s := NewSpool(dir)
	_ = s.Enqueue("ingest", []byte(`{"x":true}`))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("files=%d", len(entries))
	}
	if filepath.Ext(entries[0].Name()) != ".jsonl" {
		t.Fatalf("name=%s", entries[0].Name())
	}
	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var e SpoolEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	if e.Op != "ingest" || string(e.Body) != `{"x":true}` {
		t.Fatalf("%+v", e)
	}
	if e.EnqueuedAt.IsZero() {
		t.Fatal("missing enqueued_at")
	}
}
