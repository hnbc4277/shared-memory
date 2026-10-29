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
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/testutil"
)

func TestLoginSetsSessionCookie(t *testing.T) {
	p := testutil.TestPool(t)
	authStore := auth.NewStore(p)
	email := "cookie-" + uuid.NewString() + "@example.com"
	password := "password123456"
	if _, err := authStore.CreateAccount(context.Background(), email, password); err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewRouter(httpapi.Deps{Auth: authStore})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("login: %v status=%d", err, res.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "sm_session" && c.Value != "" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("expected sm_session cookie")
	}

	regBody, _ := json.Marshal(map[string]string{"email": "reg-" + uuid.NewString() + "@example.com", "password": "password123456"})
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/v1/auth/register", bytes.NewReader(regBody))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("register: %v status=%d", err, res.StatusCode)
	}
	var regCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "sm_session" && c.Value != "" {
			regCookie = c
			break
		}
	}
	if regCookie == nil {
		t.Fatal("expected sm_session on register")
	}

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/me", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("me with cookie: %v status=%d", err, res.StatusCode)
	}
}
