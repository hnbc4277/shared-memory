package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultDaemonURL = "http://127.0.0.1:18765"
	MaxContextChars  = 8000
	AgentCursor      = "cursor"
	AgentCodex       = "codex"
)

// API talks to the local daemon or upstream Shared Memory API.
type API interface {
	Ingest(ctx context.Context, body map[string]any) error
	Recall(ctx context.Context, query string) (string, error)
}

// HTTPAPI posts JSON with optional Bearer auth (daemon needs no key).
type HTTPAPI struct {
	BaseURL        string
	APIKey         string
	ConnectionID   string
	HTTPClient     *http.Client
}

func (c *HTTPAPI) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 8 * time.Second}
}

func (c *HTTPAPI) Ingest(ctx context.Context, body map[string]any) error {
	return c.post(ctx, "/v1/ingest", c.withConnectionID(body), nil)
}

func (c *HTTPAPI) Recall(ctx context.Context, query string) (string, error) {
	var out struct {
		Context string `json:"context"`
	}
	body := c.withConnectionID(map[string]any{"query": query, "top_k": 8, "source": "hook"})
	if err := c.post(ctx, "/v1/recall", body, &out); err != nil {
		return "", err
	}
	return out.Context, nil
}

func (c *HTTPAPI) withConnectionID(body map[string]any) map[string]any {
	if c.ConnectionID == "" {
		return body
	}
	if v, ok := body["connection_id"].(string); ok && strings.TrimSpace(v) != "" {
		return body
	}
	body["connection_id"] = c.ConnectionID
	return body
}

func (c *HTTPAPI) post(ctx context.Context, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	res, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("api %s: %s", res.Status, truncate(string(respBody), 200))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

// DaemonReachable probes GET /healthz.
func DaemonReachable(base string, client *http.Client) bool {
	if client == nil {
		client = &http.Client{Timeout: 400 * time.Millisecond}
	}
	res, err := client.Get(strings.TrimRight(base, "/") + "/healthz")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode == 200
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func TruncateContext(s string) string {
	if len(s) <= MaxContextChars {
		return s
	}
	return s[:MaxContextChars]
}

func externalSessionID(agent string, m map[string]any) string {
	for _, k := range []string{"conversation_id", "session_id", "conversationId", "sessionId"} {
		if v, ok := stringField(m, k); ok && v != "" {
			return agent + ":" + v
		}
	}
	return ""
}

func stringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case fmt.Stringer:
		return t.String(), true
	default:
		return fmt.Sprint(t), true
	}
}

func eventName(m map[string]any) string {
	for _, k := range []string{"hook_event_name", "event", "type", "hook_event"} {
		if v, ok := stringField(m, k); ok && v != "" {
			return v
		}
	}
	return ""
}
