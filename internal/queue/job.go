package queue

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

const (
	JobMessage    = "message_process"
	JobAttachment = "attachment_process"
	JobKBParse    = "kb_parse"
)

// Job carries redacted message content transiently, so semantic derivation
// works even when the account disabled transcript storage.
type Job struct {
	Type         string     `json:"type"`
	AccountID    uuid.UUID  `json:"account_id"`
	ProjectID    *uuid.UUID `json:"project_id,omitempty"`
	SessionID    uuid.UUID  `json:"session_id,omitempty"`
	MessageID    uuid.UUID  `json:"message_id,omitempty"`
	AttachmentID uuid.UUID  `json:"attachment_id,omitempty"`
	DocumentID   uuid.UUID  `json:"document_id,omitempty"`
	Role         string     `json:"role,omitempty"`
	AgentPath    string     `json:"agent_path,omitempty"`
	Content      string     `json:"content,omitempty"`
}

type Enqueuer interface {
	Enqueue(ctx context.Context, job Job) error
}

// Recorder is an Enqueuer for unit tests.
type Recorder struct {
	mu   sync.Mutex
	Jobs []Job
	Err  error
}

func (r *Recorder) Enqueue(_ context.Context, job Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.Jobs = append(r.Jobs, job)
	return nil
}
