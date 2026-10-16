package semantic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sharedmemory/internal/semantic"
)

func TestProbe_TEIAndOpenAI(t *testing.T) {
	t.Run("tei", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/embed" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode([][]float32{{0.1, 0.2}})
		}))
		defer srv.Close()
		dim, err := semantic.Probe(context.Background(), "tei", srv.URL, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if dim != 2 {
			t.Fatalf("dim = %d want 2", dim)
		}
	})

	t.Run("openai", func(t *testing.T) {
		srv := openAIFakeServer(t, "key", "m")
		defer srv.Close()
		dim, err := semantic.Probe(context.Background(), "openai", srv.URL, "m", "key")
		if err != nil {
			t.Fatal(err)
		}
		if dim != 2 {
			t.Fatalf("dim = %d want 2", dim)
		}
	})
}

func TestProbe_UnsupportedProtocol(t *testing.T) {
	_, err := semantic.Probe(context.Background(), "anthropic", "http://x", "m", "k")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `unsupported protocol "anthropic"`) {
		t.Fatalf("err = %v", err)
	}
}
