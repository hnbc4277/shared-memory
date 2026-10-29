package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/semantic"
)

type fakeEmbeddingStore struct {
	settings semantic.Settings
	lastKey  string
	keyVia   bool
}

func (f *fakeEmbeddingStore) Get(_ context.Context, _ uuid.UUID) (semantic.Settings, error) {
	return f.settings, nil
}

func (f *fakeEmbeddingStore) Upsert(_ context.Context, _ uuid.UUID, st semantic.Settings, apiKeyPlain ...string) error {
	f.settings = st
	f.keyVia = len(apiKeyPlain) > 0
	if f.keyVia {
		f.lastKey = apiKeyPlain[0]
		if apiKeyPlain[0] != "" {
			f.settings.APIKeySet = true
		} else {
			f.settings.APIKeySet = false
		}
	}
	return nil
}

func (f *fakeEmbeddingStore) Clear(_ context.Context, _ uuid.UUID) error {
	f.settings = semantic.Settings{}
	return nil
}

func (f *fakeEmbeddingStore) PlainAPIKey(_ context.Context, _ uuid.UUID) (string, error) {
	if f.settings.APIKeySet {
		return f.lastKey, nil
	}
	return "", nil
}

func TestPutEmbedding_ProbeAndReindexFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()

	accountID := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	store := &fakeEmbeddingStore{}
	router := &Router{Deps: Deps{
		Embedding:     store,
		EmbedMaxDim:   4096,
		EmbedResolver: &semantic.Resolver{Settings: store},
	}}

	put := func(model string) map[string]any {
		body := map[string]any{
			"enabled":  true,
			"protocol": "openai",
			"base_url": srv.URL,
			"model":    model,
			"api_key":  "sk",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/v1/embedding-settings", bytes.NewReader(b))
		req = req.WithContext(withAccountID(req.Context(), accountID))
		rec := httptest.NewRecorder()
		router.handlePutEmbeddingSettings(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: status=%d body=%s", model, rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	first := put("m1")
	if first["dim"] != float64(2) {
		t.Fatalf("dim=%v want 2", first["dim"])
	}
	if first["reindex_required"] != true {
		t.Fatalf("reindex_required=%v want true", first["reindex_required"])
	}
	if _, ok := first["api_key"]; ok {
		t.Fatal("response must not include api_key")
	}
	if first["api_key_set"] != true {
		t.Fatalf("api_key_set=%v want true", first["api_key_set"])
	}
	if !store.keyVia || store.lastKey != "sk" {
		t.Fatalf("expected key via variadic upsert, keyVia=%v lastKey=%q", store.keyVia, store.lastKey)
	}

	fp1 := semantic.Fingerprint("openai", "m1", 2, srv.URL)
	store.settings.ActiveFingerprint = fp1
	store.settings.ReindexRequired = false

	second := put("m2")
	if second["reindex_required"] != true {
		t.Fatalf("second reindex_required=%v want true", second["reindex_required"])
	}
	if second["dim"] != float64(2) {
		t.Fatalf("second dim=%v want 2", second["dim"])
	}
	b, _ := json.Marshal(second)
	if strings.Contains(string(b), "sk") {
		t.Fatal("response leaked api key")
	}
}
