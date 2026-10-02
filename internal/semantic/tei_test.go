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

func teiServer(t *testing.T, dim int, seen *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Inputs    string `json:"inputs"`
			Normalize bool   `json:"normalize"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !in.Normalize {
			t.Errorf("bad request: %v normalize=%v", err, in.Normalize)
		}
		*seen = append(*seen, in.Inputs)
		_ = json.NewEncoder(w).Encode([][]float32{make([]float32, dim)})
	}))
}

func TestTEIEmbedder_PrefixesQueriesOnly(t *testing.T) {
	var seen []string
	srv := teiServer(t, 4, &seen)
	defer srv.Close()
	e := semantic.NewTEIEmbedder(srv.URL, 4)

	if _, err := e.Embed(context.Background(), "正文"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.EmbedQuery(context.Background(), "问题"); err != nil {
		t.Fatal(err)
	}
	if seen[0] != "正文" {
		t.Fatalf("passage must be sent verbatim, got %q", seen[0])
	}
	if !strings.HasPrefix(seen[1], semantic.BGEQueryInstruction) || !strings.HasSuffix(seen[1], "问题") {
		t.Fatalf("query must carry the retrieval instruction, got %q", seen[1])
	}
}

func TestTEIEmbedder_RejectsWrongDimension(t *testing.T) {
	var seen []string
	srv := teiServer(t, 3, &seen)
	defer srv.Close()
	if _, err := semantic.NewTEIEmbedder(srv.URL, 4).Embed(context.Background(), "x"); err == nil {
		t.Fatal("dimension mismatch must be an error, not a silently corrupt vector")
	}
}
