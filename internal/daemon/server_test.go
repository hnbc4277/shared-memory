package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"sharedmemory/internal/cred"
)

func TestProxyIngestSuccess(t *testing.T) {
	var gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/ingest" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"session_id":"x"}`))
	}))
	defer up.Close()

	spool := NewSpool(t.TempDir())
	s := NewServer(cred.Credentials{APIKey: "k", APIURL: up.URL}, spool)
	s.HTTPClient = up.Client()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", io.NopCloser(
		jsonReader(map[string]any{"role": "user", "content": "hi", "agent": "cursor"}),
	))
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotAuth != "Bearer k" {
		t.Fatalf("auth=%q", gotAuth)
	}
	if n, _ := spool.Pending(); n != 0 {
		t.Fatalf("pending=%d", n)
	}
}

func TestProxyIngestSpooledOnFailure(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer up.Close()

	spool := NewSpool(t.TempDir())
	s := NewServer(cred.Credentials{APIKey: "k", APIURL: up.URL}, spool)
	s.HTTPClient = up.Client()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", jsonReader(map[string]any{"role": "user"}))
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["spooled"] != true {
		t.Fatalf("body=%v", body)
	}
	if n, _ := spool.Pending(); n != 1 {
		t.Fatalf("pending=%d", n)
	}
}

func TestHealthzReportsSpool(t *testing.T) {
	spool := NewSpool(t.TempDir())
	_ = spool.Enqueue("ingest", []byte(`{}`))
	s := NewServer(cred.Credentials{APIKey: "k", APIURL: "http://127.0.0.1:9"}, spool)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var h healthzResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &h)
	if !h.OK || h.SpoolPending != 1 {
		t.Fatalf("%+v", h)
	}
}

func TestHeartbeatPosts(t *testing.T) {
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/devices/heartbeat" {
			hits.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["daemon_version"] == "" {
				t.Error("missing daemon_version")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer up.Close()
	spool := NewSpool(t.TempDir())
	s := NewServer(cred.Credentials{APIKey: "k", APIURL: up.URL}, spool)
	s.HTTPClient = up.Client()
	s.heartbeat(t.Context())
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func jsonReader(v any) *jsonBody {
	b, _ := json.Marshal(v)
	return &jsonBody{data: b}
}

type jsonBody struct {
	data []byte
	off  int
}

func (j *jsonBody) Read(p []byte) (int, error) {
	if j.off >= len(j.data) {
		return 0, io.EOF
	}
	n := copy(p, j.data[j.off:])
	j.off += n
	return n, nil
}

func (j *jsonBody) Close() error { return nil }
