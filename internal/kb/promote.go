package kb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/attachment"
	"sharedmemory/internal/queue"
)

// WithSources wires artifact/attachment stores used by explicit promote.
func (s *Store) WithSources(arts *artifact.Store, atts *attachment.Service) *Store {
	s.arts = arts
	s.atts = atts
	return s
}

// AddFromArtifact copies the artifact's current version body into the KB.
func (s *Store) AddFromArtifact(ctx context.Context, accountID, artifactID uuid.UUID) (Document, error) {
	if s.arts == nil {
		return Document{}, fmt.Errorf("kb promote: artifact store not configured")
	}
	d, err := s.arts.Get(ctx, accountID, artifactID)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	body := []byte(d.Body)
	title := strings.TrimSpace(d.Artifact.Title)
	if title == "" {
		title = "untitled"
	}
	aid := d.Artifact.ID
	return s.addBytes(ctx, accountID, addBytesInput{
		Title:            title,
		MIME:             "text/markdown",
		Name:             title + ".md",
		Body:             body,
		SourceType:       SourceArtifact,
		SourceArtifactID: &aid,
	})
}

// AddFromAttachment prefers derived_text as text/plain; otherwise copies original bytes+mime.
func (s *Store) AddFromAttachment(ctx context.Context, accountID, attachmentID uuid.UUID) (Document, error) {
	if s.atts == nil {
		return Document{}, fmt.Errorf("kb promote: attachment service not configured")
	}
	a, key, err := s.atts.Get(ctx, accountID, attachmentID)
	if err != nil {
		if errors.Is(err, attachment.ErrNotFound) {
			return Document{}, ErrNotFound
		}
		return Document{}, err
	}
	aid := a.ID
	title := strings.TrimSpace(a.Filename)
	if title == "" {
		title = "untitled"
	}

	if a.DerivedText != nil && strings.TrimSpace(*a.DerivedText) != "" {
		body := []byte(*a.DerivedText)
		return s.addBytes(ctx, accountID, addBytesInput{
			Title:              title,
			MIME:               "text/plain",
			Name:               title + ".txt",
			Body:               body,
			SourceType:         SourceAttachment,
			SourceAttachmentID: &aid,
		})
	}

	if key == nil || *key == "" || !a.BinaryStored {
		return Document{}, fmt.Errorf("%w: attachment has no derived text or stored binary", ErrNotFound)
	}
	rc, err := s.objects.Get(ctx, *key)
	if err != nil {
		return Document{}, fmt.Errorf("read attachment object: %w", err)
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, MaxBytes+1))
	if err != nil {
		return Document{}, err
	}
	return s.addBytes(ctx, accountID, addBytesInput{
		Title:              title,
		MIME:               a.MIME,
		Name:               a.Filename,
		Body:               body,
		SourceType:         SourceAttachment,
		SourceAttachmentID: &aid,
	})
}

type addBytesInput struct {
	Title              string
	MIME               string
	Name               string
	Body               []byte
	SourceType         string
	SourceArtifactID   *uuid.UUID
	SourceAttachmentID *uuid.UUID
}

func (s *Store) addBytes(ctx context.Context, accountID uuid.UUID, in addBytesInput) (Document, error) {
	if len(in.Body) > MaxBytes {
		return Document{}, ErrTooLarge
	}
	m := attachment.NormalizeMIME(in.MIME, in.Name)
	if !allowedMIME[m] {
		return Document{}, fmt.Errorf("%w: %s", ErrUnsupportedType, m)
	}
	sum := sha256.Sum256(in.Body)
	hash := hex.EncodeToString(sum[:])

	existing, err := scanDoc(s.pool.QueryRow(ctx, `
		SELECT `+docColumns+` FROM kb_documents
		WHERE account_id = $1 AND content_hash = $2 AND status = $3
		LIMIT 1`, accountID, hash, StatusReady))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Document{}, err
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = strings.TrimSpace(in.Name)
		if title == "" {
			title = "untitled"
		} else {
			title = filepath.Base(strings.ReplaceAll(title, `\`, "/"))
		}
	}
	id := uuid.New()
	key := ObjectKey(accountID, id)
	if err := s.objects.Put(ctx, key, bytes.NewReader(in.Body), int64(len(in.Body)), m); err != nil {
		return Document{}, fmt.Errorf("store kb object: %w", err)
	}
	doc, err := scanDoc(s.pool.QueryRow(ctx, `
		INSERT INTO kb_documents (
			id, account_id, title, mime, source_type, source_artifact_id, source_attachment_id,
			object_key, bytes, content_hash, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+docColumns,
		id, accountID, title, m, in.SourceType, in.SourceArtifactID, in.SourceAttachmentID,
		key, int64(len(in.Body)), hash, StatusPending))
	if err != nil {
		_ = s.objects.Delete(ctx, key)
		return Document{}, fmt.Errorf("insert kb document: %w", err)
	}
	if err := s.enqueue.Enqueue(ctx, queue.Job{
		Type: queue.JobKBParse, AccountID: accountID, DocumentID: doc.ID,
	}); err != nil {
		log.Printf("enqueue kb_parse %s: %v", doc.ID, err)
	}
	return doc, nil
}
