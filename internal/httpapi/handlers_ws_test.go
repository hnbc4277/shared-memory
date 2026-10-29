//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/httpapi"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/recall"
	"sharedmemory/internal/session"
	"sharedmemory/internal/testutil"
)

func TestWS_PingWithQueryToken(t *testing.T) {
	pool := testutil.TestPool(t)
	authStore := auth.NewStore(pool)
	acct := testutil.NewAccount(t, pool)
	key, err := authStore.CreateAPIKey(context.Background(), acct, "ws")
	if err != nil {
		t.Fatal(err)
	}

	h := httpapi.NewRouter(httpapi.Deps{
		Auth:     authStore,
		Projects: project.NewStore(pool),
		Policies: policy.NewStore(pool),
		Sessions: &session.Service{Pool: pool, Policies: policy.NewStore(pool), Projects: project.NewStore(pool)},
		Recall:   &recall.Service{Pool: pool, Policies: policy.NewStore(pool)},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/v1/ws?access_token="+key, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	var ready map[string]any
	if err := wsjson.Read(ctx, conn, &ready); err != nil {
		t.Fatal(err)
	}
	if ready["type"] != "ready" {
		t.Fatalf("ready = %v", ready)
	}
	if err := wsjson.Write(ctx, conn, map[string]any{"type": "ping"}); err != nil {
		t.Fatal(err)
	}
	var pong map[string]any
	if err := wsjson.Read(ctx, conn, &pong); err != nil {
		t.Fatal(err)
	}
	if pong["type"] != "pong" {
		t.Fatalf("pong = %v", pong)
	}
}

func TestWS_PingWithWebSessionToken(t *testing.T) {
	pool := testutil.TestPool(t)
	authStore := auth.NewStore(pool)
	email := "ws-wss-" + uuid.NewString() + "@example.com"
	if _, err := authStore.CreateAccount(context.Background(), email, "password123456"); err != nil {
		t.Fatal(err)
	}
	sess, err := authStore.LoginWithPassword(context.Background(), email, "password123456")
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewRouter(httpapi.Deps{Auth: authStore})
	srv := httptest.NewServer(h)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/v1/ws?access_token="+sess.Token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	var ready map[string]any
	if err := wsjson.Read(ctx, conn, &ready); err != nil {
		t.Fatal(err)
	}
	if ready["type"] != "ready" {
		t.Fatalf("ready = %v", ready)
	}
	if err := wsjson.Write(ctx, conn, map[string]any{"type": "ping"}); err != nil {
		t.Fatal(err)
	}
	var pong map[string]any
	if err := wsjson.Read(ctx, conn, &pong); err != nil {
		t.Fatal(err)
	}
	if pong["type"] != "pong" {
		t.Fatalf("pong = %v", pong)
	}
}

func TestWS_RejectsBadToken(t *testing.T) {
	pool := testutil.TestPool(t)
	h := httpapi.NewRouter(httpapi.Deps{Auth: auth.NewStore(pool)})
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/v1/ws?access_token=sm_bad", nil)
	if err == nil {
		t.Fatal("expected dial failure")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
