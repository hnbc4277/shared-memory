package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("object not found")

// Store is the S3-compatible object storage contract (MinIO locally, OSS/S3 later).
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	ListKeys(ctx context.Context, prefix string) ([]string, error)
}

func scopeSegment(projectID *uuid.UUID) string {
	if projectID == nil {
		return "global"
	}
	return projectID.String()
}

func AccountPrefix(accountID uuid.UUID) string {
	return accountID.String() + "/"
}

func ProjectPrefix(accountID, projectID uuid.UUID) string {
	return fmt.Sprintf("%s/%s/", accountID, projectID)
}

func TranscriptKey(accountID uuid.UUID, projectID *uuid.UUID, sessionID uuid.UUID, seq int, messageID uuid.UUID) string {
	return fmt.Sprintf("%s/%s/transcripts/%s/%06d-%s.json", accountID, scopeSegment(projectID), sessionID, seq, messageID)
}

func TranscriptPrefix(accountID uuid.UUID, projectID *uuid.UUID, sessionID uuid.UUID) string {
	return fmt.Sprintf("%s/%s/transcripts/%s/", accountID, scopeSegment(projectID), sessionID)
}

func AttachmentKey(accountID uuid.UUID, projectID *uuid.UUID, attachmentID uuid.UUID, filename string) string {
	return fmt.Sprintf("%s/%s/attachments/%s/%s", accountID, scopeSegment(projectID), attachmentID, filename)
}

func ThumbnailKey(accountID uuid.UUID, projectID *uuid.UUID, attachmentID uuid.UUID) string {
	return fmt.Sprintf("%s/%s/attachments/%s/thumb.jpg", accountID, scopeSegment(projectID), attachmentID)
}

func ArtifactCandidateKey(accountID uuid.UUID, projectID *uuid.UUID, candidateID uuid.UUID) string {
	return fmt.Sprintf("%s/%s/artifact-candidates/%s.md", accountID, scopeSegment(projectID), candidateID)
}

func ArtifactVersionKey(accountID uuid.UUID, projectID *uuid.UUID, artifactID uuid.UUID, version int) string {
	return fmt.Sprintf("%s/%s/artifacts/%s/v%04d.md", accountID, scopeSegment(projectID), artifactID, version)
}
