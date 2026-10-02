package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SpoolEntry is one offline request queued as a JSONL line.
type SpoolEntry struct {
	Op         string          `json:"op"` // ingest | recall
	Body       json.RawMessage `json:"body"`
	EnqueuedAt time.Time       `json:"enqueued_at"`
}

// Spool stores failed upstream POSTs under dir/*.jsonl.
type Spool struct {
	dir string
	mu  sync.Mutex
}

func NewSpool(dir string) *Spool {
	return &Spool{dir: dir}
}

func (s *Spool) Dir() string { return s.dir }

func (s *Spool) Ensure() error {
	return os.MkdirAll(s.dir, 0o700)
}

func (s *Spool) Enqueue(op string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Ensure(); err != nil {
		return err
	}
	e := SpoolEntry{
		Op:         op,
		Body:       json.RawMessage(append([]byte(nil), body...)),
		EnqueuedAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%d-%d.jsonl", time.Now().UnixNano(), os.Getpid())
	path := filepath.Join(s.dir, name)
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func (s *Spool) Pending() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.listLocked()
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return len(files), nil
}

func (s *Spool) listLocked() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".jsonl") {
			out = append(out, filepath.Join(s.dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Replay posts queued entries oldest-first. Stops on first failure so order is preserved.
func (s *Spool) Replay(client *http.Client, apiURL, apiKey string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := s.listLocked()
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(apiURL, "/")
	done := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return done, err
		}
		line := bytes.TrimSpace(raw)
		var e SpoolEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return done, err
		}
		pathURL := base + "/v1/" + e.Op
		req, err := http.NewRequest(http.MethodPost, pathURL, bytes.NewReader(e.Body))
		if err != nil {
			return done, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		res, err := client.Do(req)
		if err != nil {
			return done, nil // keep spool; network flake
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
			return done, nil
		}
		if res.StatusCode >= 400 {
			// Drop permanently failing entries so they don't block the queue.
			_ = os.Remove(path)
			done++
			continue
		}
		if err := os.Remove(path); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}
