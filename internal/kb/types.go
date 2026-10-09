package kb

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	SourceUpload     = "upload"
	SourceArtifact   = "artifact"
	SourceAttachment = "attachment"
	StatusPending    = "pending"
	StatusReady      = "ready"
	StatusFailed     = "failed"

	MaxBytes        = 20 << 20
	DefaultMaxRunes = 1200
	docxMIME        = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

var (
	ErrNotFound        = errors.New("kb document not found")
	ErrUnsupportedType = errors.New("unsupported kb document type")
	ErrTooLarge        = errors.New("kb document exceeds 20MB")
)

var allowedMIME = map[string]bool{
	"text/plain":      true,
	"text/markdown":   true,
	"application/pdf": true,
	docxMIME:          true,
}

type Document struct {
	ID                 uuid.UUID  `json:"id"`
	Title              string     `json:"title"`
	MIME               string     `json:"mime"`
	SourceType         string     `json:"source_type"`
	SourceArtifactID   *uuid.UUID `json:"source_artifact_id,omitempty"`
	SourceAttachmentID *uuid.UUID `json:"source_attachment_id,omitempty"`
	Bytes              int64      `json:"bytes"`
	ContentHash        string     `json:"content_hash"`
	Status             string     `json:"status"`
	Error              string     `json:"error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type Chunk struct {
	ID         uuid.UUID
	DocumentID uuid.UUID
	Ordinal    int
	Body       string
	CharStart  *int
	CharEnd    *int
}

type AddUploadInput struct {
	Title string
	MIME  string
	Name  string // filename for mime sniff
	Body  []byte
}
