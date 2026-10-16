package semantic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sharedmemory/internal/semantic"
)

func openAIFakeServer(t *testing.T, wantAuth string, wantModel string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/embeddings") {
			t.Errorf("path: got %q want suffix /v1/embeddings", r.URL.Path)
		}
		if wantAuth != "" {
			if got := r.Header.Get("Authorization"); got != "Bearer "+wantAuth {
				t.Errorf("Authorization: got %q want Bearer %q", got, wantAuth)
			}
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var in struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.Unmarshal(b, &in); err != nil {
			t.Fatalf("body json: %v", err)
		}
		if in.Model != wantModel || in.Input != "probe text" {
			t.Errorf("body: model=%q input=%q", in.Model, in.Input)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float64{0.1, 0.2}},
			},
		})
	}))
}

func TestOpenAIEmbedder_EmbedAndDim(t *testing.T) {
	t.Run("base without v1", func(t *testing.T) {
		srv := openAIFakeServer(t, "secret", "text-embedding-3-small")
		defer srv.Close()
		e := semantic.NewOpenAIEmbedder(srv.URL, "secret", "text-embedding-3-small", 2)
		if e.Dim() != 2 {
			t.Fatalf("Dim() = %d want 2", e.Dim())
		}
		vec, err := e.Embed(context.Background(), "probe text")
		if err != nil {
			t.Fatal(err)
		}
		if len(vec) != 2 || vec[0] != 0.1 || vec[1] != 0.2 {
			t.Fatalf("vec = %v", vec)
		}
	})

	t.Run("base ending /v1", func(t *testing.T) {
		srv := openAIFakeServer(t, "secret", "text-embedding-3-small")
		defer srv.Close()
		e := semantic.NewOpenAIEmbedder(srv.URL+"/v1", "secret", "text-embedding-3-small", 2)
		vec, err := e.Embed(context.Background(), "probe text")
		if err != nil {
			t.Fatal(err)
		}
		if len(vec) != 2 {
			t.Fatalf("len(vec) = %d want 2", len(vec))
		}
	})
}
