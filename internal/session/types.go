package session

import (
	"time"

	"github.com/google/uuid"
)

// IngestMessageRequest is the wire contract every adapter (REST, MCP, official
// plugins) uses to report one chat message. Skills/Plugins/Tools are pointers
// so a missing JSON key can be told apart from an explicit empty array.
type IngestMessageRequest struct {
	SessionID           string    `json:"session_id"`
	ExternalSessionID   string    `json:"external_session_id"`
	ProjectID           string    `json:"project_id"`
	Title               string    `json:"title"`
	Agent               string    `json:"agent"`
	AgentPath           string    `json:"agent_path"`
	Skills              *[]string `json:"skills"`
	Plugins             *[]string `json:"plugins"`
	Tools               *[]string  `json:"tools"`
	ToolCalls           []ToolCall `json:"tool_calls"`
	Model               string     `json:"model"`
	Role                string     `json:"role"`
	Content             string     `json:"content"`
	AttachmentIDs       []string   `json:"attachment_ids"`
	StoreTranscriptMark bool       `json:"store_transcript_mark"`
	ArtifactOffer       bool       `json:"artifact_offer"`
	ConnectionID        string     `json:"connection_id,omitempty"`
}

// ToolCall records one plugin/tool invocation with timing and truncated output.
type ToolCall struct {
	Kind           string `json:"kind"` // plugin | tool
	Name           string `json:"name"`
	StartedAt      string `json:"started_at"` // RFC3339
	EndedAt        string `json:"ended_at"`
	Output         string `json:"output"`
	OutputTruncated bool  `json:"-"`
}

type Prompt struct {
	Kind        string `json:"kind"` // "vault" | "artifact"
	CandidateID string `json:"candidate_id"`
	Text        string `json:"text"`
}

type IngestResult struct {
	SessionID uuid.UUID `json:"session_id"`
	MessageID uuid.UUID `json:"message_id"`
	Seq       int       `json:"seq"`
	Warnings  []string  `json:"warnings"`
	Prompts   []Prompt  `json:"prompts"`
}

type Session struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    *uuid.UUID `json:"project_id"`
	Agent        string     `json:"agent"`
	AgentPath    string     `json:"agent_path"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	LastActiveAt time.Time  `json:"last_active_at"`
	MessageCount int        `json:"message_count"`
	AgentPaths   []string   `json:"agent_paths"`
	Degraded     bool       `json:"degraded"`
}

type Message struct {
	ID            uuid.UUID  `json:"id"`
	Seq           int        `json:"seq"`
	Role          string     `json:"role"`
	Agent         string     `json:"agent"`
	AgentPath     string     `json:"agent_path"`
	Skills        []string   `json:"skills"`
	Plugins       []string   `json:"plugins"`
	Tools         []string   `json:"tools"`
	ToolCalls     []ToolCall `json:"tool_calls,omitempty"`
	Model         string     `json:"model"`
	Content       *string    `json:"content"`
	ContentStored bool       `json:"content_stored"`
	AttachmentIDs []string   `json:"attachment_ids"`
	CreatedAt     time.Time  `json:"created_at"`
}

type SessionDetail struct {
	Session  Session   `json:"session"`
	Messages []Message `json:"messages"`
}

// TranscriptEntry is the object written to the object store per message.
type TranscriptEntry struct {
	MessageID     uuid.UUID `json:"message_id"`
	SessionID     uuid.UUID `json:"session_id"`
	Seq           int       `json:"seq"`
	Role          string    `json:"role"`
	Agent         string    `json:"agent"`
	AgentPath     string    `json:"agent_path"`
	Skills        []string  `json:"skills"`
	Plugins       []string  `json:"plugins"`
	Tools         []string  `json:"tools"`
	Model         string    `json:"model"`
	Content       string    `json:"content"`
	AttachmentIDs []string  `json:"attachment_ids"`
	CreatedAt     time.Time `json:"created_at"`
}
