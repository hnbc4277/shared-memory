//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/entity"
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/testutil"
)

func TestEntitiesCRUD(t *testing.T) {
	p := testutil.TestPool(t)
	acct := testutil.NewAccount(t, p)
	authStore := auth.NewStore(p)
	key, err := authStore.CreateAPIKey(context.Background(), acct, "entities")
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewRouter(httpapi.Deps{
		Auth:     authStore,
		Entities: &entity.Store{Pool: p},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	createBody, _ := json.Marshal(map[string]string{"canonical": "OpenSearch", "alias": "OS"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/entities", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("create: %v status=%d", err, res.StatusCode)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/entities", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("list: %v status=%d", err, res.StatusCode)
	}

	patchBody, _ := json.Marshal(map[string]string{"canonical": "OpenSearch 2", "alias": "OS2"})
	req, _ = http.NewRequest(http.MethodPatch, srv.URL+"/v1/entities/"+created.ID.String(), bytes.NewReader(patchBody))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("patch: %v status=%d", err, res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/v1/entities/"+created.ID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+key)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %v status=%d", err, res.StatusCode)
	}
}
