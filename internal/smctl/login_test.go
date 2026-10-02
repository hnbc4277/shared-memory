package smctl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"sharedmemory/internal/cred"
)

func TestLoginSavesCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHARED_MEMORY_HOME", home)

	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "dc",
				"user_code":                 "ABCD2345",
				"verification_uri":          "http://example/device",
				"verification_uri_complete": "http://example/device?user_code=ABCD2345",
				"expires_in":                600,
				"interval":                  1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/device/token":
			n := polls.Add(1)
			if n < 2 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_key":    "sm_live",
				"device_id":  "11111111-1111-1111-1111-111111111111",
				"account_id": "22222222-2222-2222-2222-222222222222",
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	var out strings.Builder
	ctx := t.Context()
	if err := Login(ctx, srv.URL, "test", "cursor", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ABCD2345") {
		t.Fatalf("output=%s", out.String())
	}
	c, err := cred.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "sm_live" || c.APIURL != srv.URL {
		t.Fatalf("%+v", c)
	}
}

func TestLogoutDeletes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHARED_MEMORY_HOME", home)
	if err := cred.Save(cred.Credentials{APIKey: "k", APIURL: "http://x", DeviceID: "d"}); err != nil {
		t.Fatal(err)
	}
	if err := Logout(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := cred.Load(); !cred.IsNotFound(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestStatusWithoutCreds(t *testing.T) {
	t.Setenv("SHARED_MEMORY_HOME", t.TempDir())
	var out strings.Builder
	if err := Status(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "missing") {
		t.Fatalf("%s", out.String())
	}
}
