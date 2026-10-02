package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"sharedmemory/internal/cred"
)

const (
	DefaultListen   = "127.0.0.1:18765"
	DefaultVersion  = "0.1.0"
	replayInterval  = 5 * time.Second
	heartbeatEvery  = 30 * time.Second
	upstreamTimeout = 30 * time.Second
)

// Server is the localhost proxy with offline spool.
type Server struct {
	ListenAddr  string
	Version     string
	Credentials cred.Credentials
	Spool       *Spool
	HTTPClient  *http.Client
	Logger      *log.Logger

	mux *http.ServeMux
	srv *http.Server
}

func NewServer(c cred.Credentials, spool *Spool) *Server {
	s := &Server{
		ListenAddr:  DefaultListen,
		Version:     DefaultVersion,
		Credentials: c,
		Spool:       spool,
		HTTPClient:  &http.Client{Timeout: upstreamTimeout},
		Logger:      log.Default(),
		mux:         http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /v1/ingest", s.handleProxy("ingest"))
	s.mux.HandleFunc("POST /v1/recall", s.handleProxy("recall"))
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

type healthzResponse struct {
	OK           bool `json:"ok"`
	SpoolPending int  `json:"spool_pending"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	n, _ := s.Spool.Pending()
	writeJSON(w, http.StatusOK, healthzResponse{OK: true, SpoolPending: n})
}

func (s *Server) handleProxy(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		up, err := s.forward(r.Context(), op, body)
		if err != nil || up == nil {
			_ = s.Spool.Enqueue(op, body)
			writeJSON(w, http.StatusAccepted, map[string]any{
				"warnings": []string{"spooled: upstream unavailable"},
				"spooled":  true,
			})
			return
		}
		defer up.Body.Close()
		respBody, _ := io.ReadAll(up.Body)
		if up.StatusCode >= 500 {
			_ = s.Spool.Enqueue(op, body)
			writeJSON(w, http.StatusAccepted, map[string]any{
				"warnings": []string{"spooled: upstream error"},
				"spooled":  true,
			})
			return
		}
		for k, vals := range up.Header {
			if strings.EqualFold(k, "Content-Length") {
				continue
			}
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		if ct := w.Header().Get("Content-Type"); ct == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(up.StatusCode)
		_, _ = w.Write(respBody)
	}
}

func (s *Server) forward(ctx context.Context, op string, body []byte) (*http.Response, error) {
	base := strings.TrimRight(s.Credentials.APIURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/"+op, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Credentials.APIKey)
	return s.HTTPClient.Do(req)
}

func (s *Server) Start(ctx context.Context) error {
	if err := s.Spool.Ensure(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", s.ListenAddr)
	if err != nil {
		return err
	}
	s.srv = &http.Server{Handler: s.mux, ReadHeaderTimeout: 10 * time.Second}
	go s.loop(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdown)
	}()
	s.Logger.Printf("shared-memory daemon listening on http://%s", s.ListenAddr)
	err = s.srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) loop(ctx context.Context) {
	replayTick := time.NewTicker(replayInterval)
	hbTick := time.NewTicker(heartbeatEvery)
	defer replayTick.Stop()
	defer hbTick.Stop()
	s.heartbeat(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-replayTick.C:
			_, _ = s.Spool.Replay(s.HTTPClient, s.Credentials.APIURL, s.Credentials.APIKey)
		case <-hbTick.C:
			s.heartbeat(ctx)
		}
	}
}

func (s *Server) heartbeat(ctx context.Context) {
	n, _ := s.Spool.Pending()
	base := strings.TrimRight(s.Credentials.APIURL, "/")
	body, _ := json.Marshal(map[string]any{
		"daemon_version": s.Version,
		"spool_pending":  n,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/devices/heartbeat", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Credentials.APIKey)
	res, err := s.HTTPClient.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
