package attachment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
)

const MaxBytes = 20 << 20

var (
	ErrNotFound        = errors.New("attachment not found")
	ErrNotStored       = errors.New("attachment binary not stored (store_attachments is off)")
	ErrUnsupportedType = errors.New("unsupported attachment type")
	ErrTooLarge        = errors.New("attachment exceeds 20MB")
)

var allowedMIME = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"application/pdf": true, "text/plain": true, "text/markdown": true, "text/csv": true,
	"application/json": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
}

// IsText reports whether derived text can be extracted without OCR/parsers.
func IsText(m string) bool {
	return strings.HasPrefix(m, "text/") || m == "application/json"
}

func NormalizeMIME(m, filename string) string {
	if mt, _, err := mime.ParseMediaType(m); err == nil {
		m = mt
	}
	if m == "" || m == "application/octet-stream" {
		switch strings.ToLower(filepath.Ext(filename)) {
		case ".md", ".markdown":
			return "text/markdown"
		case ".txt", ".log":
			return "text/plain"
		case ".csv":
			return "text/csv"
		case ".json":
			return "application/json"
		}
		if byExt := mime.TypeByExtension(filepath.Ext(filename)); byExt != "" {
			mt, _, _ := mime.ParseMediaType(byExt)
			return mt
		}
	}
	return m
}

type Attachment struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    *uuid.UUID `json:"project_id"`
	SessionID    *uuid.UUID `json:"session_id"`
	MessageID    *uuid.UUID `json:"message_id"`
	AgentPath    string     `json:"agent_path"`
	Filename     string     `json:"filename"`
	MIME         string     `json:"mime"`
	SizeBytes    int64      `json:"size_bytes"`
	ContentHash  string     `json:"content_hash"`
	BinaryStored bool       `json:"binary_stored"`
	DerivedText  *string    `json:"derived_text,omitempty"`
	HasThumbnail bool       `json:"has_thumbnail"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Upload struct {
	ProjectID *uuid.UUID
	AgentPath string
	Filename  string
	MIME      string
	Body      io.Reader
}

type Service struct {
	Pool     *pgxpool.Pool
	Objects  objectstore.Store
	Policies *policy.Store
	Projects *project.Store
	Queue    queue.Enqueuer
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

func safeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = unsafeName.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == "_" {
		return "file"
	}
	return name
}

func (s *Service) Upload(ctx context.Context, accountID uuid.UUID, u Upload) (Attachment, []string, error) {
	warnings := []string{}
	if u.ProjectID != nil {
		if _, err := s.Projects.Get(ctx, accountID, *u.ProjectID); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return Attachment{}, nil, fmt.Errorf("%w: project", ErrNotFound)
			}
			return Attachment{}, nil, err
		}
	}
	filename := safeFilename(u.Filename)
	m := NormalizeMIME(u.MIME, filename)
	if !allowedMIME[m] {
		return Attachment{}, nil, fmt.Errorf("%w: %s", ErrUnsupportedType, m)
	}
	data, err := io.ReadAll(io.LimitReader(u.Body, MaxBytes+1))
	if err != nil {
		return Attachment{}, nil, err
	}
	if len(data) > MaxBytes {
		return Attachment{}, nil, ErrTooLarge
	}
	pol, err := s.Policies.Get(ctx, accountID)
	if err != nil {
		return Attachment{}, nil, err
	}
	sum := sha256.Sum256(data)
	a := Attachment{ID: uuid.New(), ProjectID: u.ProjectID, AgentPath: strings.TrimSpace(u.AgentPath),
		Filename: filename, MIME: m, SizeBytes: int64(len(data)), ContentHash: hex.EncodeToString(sum[:])}
	var key *string
	var thumbKey *string
	if pol.StoreAttachments {
		k := objectstore.AttachmentKey(accountID, u.ProjectID, a.ID, filename)
		if err := s.Objects.Put(ctx, k, bytes.NewReader(data), int64(len(data)), m); err != nil {
			return Attachment{}, nil, fmt.Errorf("store attachment: %w", err)
		}
		key = &k
		a.BinaryStored = true
		if thumb, ok := MakeThumbnail(m, data); ok {
			tk := objectstore.ThumbnailKey(accountID, u.ProjectID, a.ID)
			if err := s.Objects.Put(ctx, tk, bytes.NewReader(thumb), int64(len(thumb)), "image/jpeg"); err != nil {
				log.Printf("thumbnail put: %v", err)
			} else {
				thumbKey = &tk
				a.HasThumbnail = true
			}
		}
	} else {
		warnings = append(warnings, "attachment_binary_not_stored")
	}
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO attachments (id, account_id, project_id, agent_path, filename, mime, size_bytes, content_hash,
			object_key, binary_stored, thumbnail_object_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING created_at`,
		a.ID, accountID, a.ProjectID, a.AgentPath, a.Filename, a.MIME, a.SizeBytes, a.ContentHash, key, a.BinaryStored, thumbKey,
	).Scan(&a.CreatedAt)
	if err != nil {
		if key != nil {
			_ = s.Objects.Delete(ctx, *key)
		}
		if thumbKey != nil {
			_ = s.Objects.Delete(ctx, *thumbKey)
		}
		return Attachment{}, nil, fmt.Errorf("insert attachment: %w", err)
	}
	if a.BinaryStored {
		err := s.Queue.Enqueue(ctx, queue.Job{Type: queue.JobAttachment, AccountID: accountID, ProjectID: a.ProjectID,
			AttachmentID: a.ID, AgentPath: a.AgentPath})
		if err != nil {
			log.Printf("enqueue attachment: %v", err)
			warnings = append(warnings, "queue_unavailable")
		}
	}
	return a, warnings, nil
}

func (s *Service) Get(ctx context.Context, accountID, id uuid.UUID) (Attachment, *string, error) {
	var a Attachment
	var key *string
	var thumb *string
	err := s.Pool.QueryRow(ctx, `
		SELECT id, project_id, session_id, message_id, COALESCE(agent_path,''), COALESCE(filename,''),
		       COALESCE(mime,''), COALESCE(size_bytes,0), COALESCE(content_hash,''), binary_stored, derived_text,
		       created_at, object_key, thumbnail_object_key
		FROM attachments WHERE id = $1 AND account_id = $2`, id, accountID,
	).Scan(&a.ID, &a.ProjectID, &a.SessionID, &a.MessageID, &a.AgentPath, &a.Filename, &a.MIME, &a.SizeBytes,
		&a.ContentHash, &a.BinaryStored, &a.DerivedText, &a.CreatedAt, &key, &thumb)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, nil, ErrNotFound
	}
	a.HasThumbnail = thumb != nil && *thumb != ""
	return a, key, err
}

func (s *Service) ThumbnailURL(ctx context.Context, accountID, id uuid.UUID) (string, error) {
	var key *string
	err := s.Pool.QueryRow(ctx, `
		SELECT thumbnail_object_key FROM attachments WHERE id = $1 AND account_id = $2`, id, accountID,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if key == nil || *key == "" {
		return "", ErrNotStored
	}
	return s.Objects.PresignGet(ctx, *key, 15*time.Minute)
}

func (s *Service) DownloadURL(ctx context.Context, accountID, id uuid.UUID) (Attachment, string, error) {
	a, key, err := s.Get(ctx, accountID, id)
	if err != nil {
		return Attachment{}, "", err
	}
	if key == nil || !a.BinaryStored {
		return a, "", ErrNotStored
	}
	url, err := s.Objects.PresignGet(ctx, *key, 15*time.Minute)
	return a, url, err
}
