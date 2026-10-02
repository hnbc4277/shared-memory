package index

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	DocMemory   = "memory"
	DocArtifact = "artifact"
)

// Doc is the projection written to both search engines. It never carries
// vault secrets; callers index only redacted, non-sensitive text.
type Doc struct {
	ID              uuid.UUID  `json:"id"`
	DocType         string     `json:"doc_type"`
	AccountID       uuid.UUID  `json:"account_id"`
	ProjectID       *uuid.UUID `json:"project_id,omitempty"`
	Scope           string     `json:"scope"`
	Kind            string     `json:"kind"`
	Body            string     `json:"body"`
	SourceSessionID *uuid.UUID `json:"source_session_id,omitempty"`
	SourceAgentPath string     `json:"source_agent_path"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Hit struct {
	ID      uuid.UUID
	DocType string
	Score   float64
}

// Query scopes every search to one account; ProjectID additionally admits
// project-scoped docs of that project next to global docs.
type Query struct {
	AccountID uuid.UUID
	ProjectID *uuid.UUID
	Text      string
	Vector    []float32
	Limit     int
}

type TextIndex interface {
	Ensure(ctx context.Context) error
	Index(ctx context.Context, d Doc) error
	Search(ctx context.Context, q Query) ([]Hit, error)
	Delete(ctx context.Context, accountID, id uuid.UUID) error
	DeleteProject(ctx context.Context, accountID, projectID uuid.UUID) error
	DeleteAccount(ctx context.Context, accountID uuid.UUID) error
}

type VectorIndex interface {
	Ensure(ctx context.Context) error
	Upsert(ctx context.Context, d Doc, vec []float32) error
	Search(ctx context.Context, q Query) ([]Hit, error)
	Delete(ctx context.Context, accountID, id uuid.UUID) error
	DeleteProject(ctx context.Context, accountID, projectID uuid.UUID) error
	DeleteAccount(ctx context.Context, accountID uuid.UUID) error
}
