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

func TestLLMDeriver_FallsBackOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	d := semantic.NewLLMDeriver(srv.URL+"/v1", "k", "m")
	sum, err := d.Summarize(context.Background(), "我们决定采用 OpenSearch 做全文检索")
	if err != nil || sum == "" {
		t.Fatalf("fallback summarize: %v %q", err, sum)
	}
	items, err := d.Extract(context.Background(), "user", "我喜欢深色主题", true)
	if err != nil || len(items) == 0 {
		t.Fatalf("fallback extract: %v %#v", err, items)
	}
}

func TestLLMDeriver_UsesModelJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{
					"content": `[{"kind":"decision","scope":"project","body":"采用 Qdrant"}]`,
				}},
			},
		})
	}))
	defer srv.Close()
	d := semantic.NewLLMDeriver(srv.URL+"/v1", "k", "m")
	items, err := d.Extract(context.Background(), "assistant", "我们决定用 Qdrant", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != semantic.KindDecision || !strings.Contains(items[0].Body, "Qdrant") {
		t.Fatalf("%#v", items)
	}
}
