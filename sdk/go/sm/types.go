package sm

import "encoding/json"

// IngestRequest is POST /v1/ingest body.
type IngestRequest struct {
	SessionID         string   `json:"session_id,omitempty"`
	ExternalSessionID string   `json:"external_session_id,omitempty"`
	ProjectID         string   `json:"project_id,omitempty"`
	Title             string   `json:"title,omitempty"`
	Agent             string   `json:"agent,omitempty"`
	AgentPath         string   `json:"agent_path,omitempty"`
	Skills            []string `json:"skills"`
	Plugins           []string `json:"plugins"`
	Tools             []string `json:"tools"`
	Model             string   `json:"model,omitempty"`
	Role              string   `json:"role"`
	Content           string   `json:"content"`
	AttachmentIDs     []string `json:"attachment_ids,omitempty"`
}

// IngestResult is the ingest response.
type IngestResult struct {
	SessionID string          `json:"session_id"`
	MessageID string          `json:"message_id"`
	Seq       int             `json:"seq"`
	Warnings  []string        `json:"warnings"`
	Prompts   []json.RawMessage `json:"prompts"`
}

// RecallRequest is POST /v1/recall body.
type RecallRequest struct {
	Query     string `json:"query"`
	ProjectID string `json:"project_id,omitempty"`
	TopK      int    `json:"top_k,omitempty"`
}

// RecallResult is the recall response.
type RecallResult struct {
	Profile        string          `json:"profile"`
	RunningSummary string          `json:"running_summary"`
	Hits           []json.RawMessage `json:"hits"`
	Context        string          `json:"context"`
	InjectEnabled  bool            `json:"inject_enabled"`
	Warnings       []string        `json:"warnings"`
}

// Session is a list-sessions item.
type Session struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Agent        string `json:"agent"`
	AgentPath    string `json:"agent_path"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	StartedAt    string `json:"started_at"`
	LastActiveAt string `json:"last_active_at"`
	MessageCount int    `json:"message_count"`
}

// SessionDetail is GET /v1/sessions/{id}.
type SessionDetail struct {
	Session  Session           `json:"session"`
	Messages []json.RawMessage `json:"messages"`
}

// Candidates is GET /v1/candidates.
type Candidates struct {
	Vault     []json.RawMessage `json:"vault"`
	Artifacts []json.RawMessage `json:"artifacts"`
}

// DeviceCode is POST /v1/device/code response.
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceToken is POST /v1/device/token success response.
type DeviceToken struct {
	APIKey    string `json:"api_key"`
	DeviceID  string `json:"device_id"`
	AccountID string `json:"account_id"`
}
