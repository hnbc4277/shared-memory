// Package sm is the Shared Memory HTTP client for Go adapters and tools.
// Configure with SHARED_MEMORY_API_URL and SHARED_MEMORY_API_KEY (or NewClient).
package sm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvAPIURL        = "SHARED_MEMORY_API_URL"
	EnvAPIKey        = "SHARED_MEMORY_API_KEY"
	EnvConnectionID  = "SHARED_MEMORY_CONNECTION_ID"
)

func mergeConnectionID(connectionID string) string {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID != "" {
		return connectionID
	}
	return strings.TrimSpace(os.Getenv(EnvConnectionID))
}

// Client talks to the Shared Memory REST API with Bearer auth.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewClient builds a client. Empty baseURL/apiKey fall back to env vars.
func NewClient(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = os.Getenv(EnvAPIURL)
	}
	if apiKey == "" {
		apiKey = os.Getenv(EnvAPIKey)
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FromEnv is NewClient("", "").
func FromEnv() *Client {
	return NewClient("", "")
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// APIError is a non-2xx response from the API.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("shared memory api: %d %s", e.Status, strings.TrimSpace(e.Body))
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	raw, err := c.CallRaw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// CallRaw performs an authenticated request and returns the response body.
// Empty bodies become {"ok":true} so MCP and adapters can treat them uniformly.
func (c *Client) CallRaw(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(raw)}
	}
	if len(raw) == 0 {
		raw = []byte(`{"ok":true}`)
	}
	return raw, nil
}

// Ingest records one chat message.
func (c *Client) Ingest(ctx context.Context, req IngestRequest) (IngestResult, error) {
	// API requires skills/plugins/tools as JSON arrays (never omitted/null).
	if req.Skills == nil {
		req.Skills = []string{}
	}
	if req.Plugins == nil {
		req.Plugins = []string{}
	}
	if req.Tools == nil {
		req.Tools = []string{}
	}
	if cid := mergeConnectionID(req.ConnectionID); cid != "" {
		req.ConnectionID = cid
	}
	var out IngestResult
	err := c.do(ctx, http.MethodPost, "/v1/ingest", req, &out)
	return out, err
}

// Recall retrieves injectable memory for a query.
func (c *Client) Recall(ctx context.Context, req RecallRequest) (RecallResult, error) {
	if cid := mergeConnectionID(req.ConnectionID); cid != "" {
		req.ConnectionID = cid
	}
	var out RecallResult
	err := c.do(ctx, http.MethodPost, "/v1/recall", req, &out)
	return out, err
}

// ListSessionsFilter optional query params for ListSessions.
type ListSessionsFilter struct {
	ProjectID string
	Agent     string
	AgentPath string
	From      string // RFC3339
	To        string // RFC3339
	Limit     int
	Offset    int
}

// ListSessions lists chat sessions for the authenticated account.
func (c *Client) ListSessions(ctx context.Context, f ListSessionsFilter) ([]Session, error) {
	q := url.Values{}
	if f.ProjectID != "" {
		q.Set("project_id", f.ProjectID)
	}
	if f.Agent != "" {
		q.Set("agent", f.Agent)
	}
	if f.AgentPath != "" {
		q.Set("agent_path", f.AgentPath)
	}
	if f.From != "" {
		q.Set("from", f.From)
	}
	if f.To != "" {
		q.Set("to", f.To)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", strconv.Itoa(f.Offset))
	}
	path := "/v1/sessions"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []Session
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetSession returns one session with messages.
func (c *Client) GetSession(ctx context.Context, sessionID string) (SessionDetail, error) {
	var out SessionDetail
	err := c.do(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID), nil, &out)
	return out, err
}

// ListCandidates returns pending vault and artifact candidates.
func (c *Client) ListCandidates(ctx context.Context) (Candidates, error) {
	var out Candidates
	err := c.do(ctx, http.MethodGet, "/v1/candidates", nil, &out)
	return out, err
}

// ResolveCandidate accepts or rejects a vault/artifact candidate.
// kind is "vault" or "artifact"; decision is "accept" or "reject".
func (c *Client) ResolveCandidate(ctx context.Context, kind, candidateID, decision string) (json.RawMessage, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	decision = strings.ToLower(strings.TrimSpace(decision))
	if kind != "vault" && kind != "artifact" {
		return nil, fmt.Errorf("kind must be vault or artifact")
	}
	if decision != "accept" && decision != "reject" {
		return nil, fmt.Errorf("decision must be accept or reject")
	}
	if strings.TrimSpace(candidateID) == "" {
		return nil, fmt.Errorf("candidate_id is required")
	}
	prefix := "/v1/vault/candidates/"
	if kind == "artifact" {
		prefix = "/v1/artifacts/candidates/"
	}
	path := prefix + url.PathEscape(candidateID) + "/" + decision
	var out json.RawMessage
	err := c.do(ctx, http.MethodPost, path, map[string]any{}, &out)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = json.RawMessage(`{"ok":true}`)
	}
	return out, nil
}

// DeviceCodeRequest starts a device-code login (unauthenticated).
type DeviceCodeRequest struct {
	ClientName string `json:"client_name"`
	Agent      string `json:"agent,omitempty"`
}

// CreateDeviceCode posts POST /v1/device/code (no Bearer required).
func (c *Client) CreateDeviceCode(ctx context.Context, req DeviceCodeRequest) (DeviceCode, error) {
	var out DeviceCode
	err := c.do(ctx, http.MethodPost, "/v1/device/code", req, &out)
	return out, err
}

// PollDeviceToken posts POST /v1/device/token until approved or error.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (DeviceToken, error) {
	var out DeviceToken
	err := c.do(ctx, http.MethodPost, "/v1/device/token", map[string]string{"device_code": deviceCode}, &out)
	return out, err
}

// Heartbeat posts POST /v1/devices/heartbeat for a device-bound API key.
func (c *Client) Heartbeat(ctx context.Context, daemonVersion string, spoolPending int) error {
	return c.do(ctx, http.MethodPost, "/v1/devices/heartbeat", map[string]any{
		"daemon_version": daemonVersion,
		"spool_pending":  spoolPending,
	}, nil)
}
