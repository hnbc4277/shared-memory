package kb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/attachment"
	"sharedmemory/internal/index"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/queue"
)

type Store struct {
	pool    *pgxpool.Pool
	objects objectstore.Store
	enqueue queue.Enqueuer
	index   *index.Service
}

func NewStore(pool *pgxpool.Pool, objects objectstore.Store, eq queue.Enqueuer, idx *index.Service) *Store {
	return &Store{pool: pool, objects: objects, enqueue: eq, index: idx}
}

func ObjectKey(accountID, docID uuid.UUID) string {
	return fmt.Sprintf("%s/kb/%s", accountID, docID)
}

const docColumns = `id, COALESCE(title,''), COALESCE(mime,''), source_type, source_artifact_id, source_attachment_id,
	COALESCE(bytes,0), COALESCE(content_hash,''), status, COALESCE(error,''), created_at, updated_at`

func scanDoc(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.Title, &d.MIME, &d.SourceType, &d.SourceArtifactID, &d.SourceAttachmentID,
		&d.Bytes, &d.ContentHash, &d.Status, &d.Error, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *Store) AddUpload(ctx context.Context, accountID uuid.UUID, in AddUploadInput) (Document, error) {
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
			id, account_id, title, mime, source_type, object_key, bytes, content_hash, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING `+docColumns,
		id, accountID, title, m, SourceUpload, key, int64(len(in.Body)), hash, StatusPending))
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

func (s *Store) List(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]Document, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+docColumns+` FROM kb_documents
		WHERE account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, accountID, id uuid.UUID) (Document, error) {
	d, err := scanDoc(s.pool.QueryRow(ctx, `
		SELECT `+docColumns+` FROM kb_documents
		WHERE id = $1 AND account_id = $2`, id, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	return d, err
}

func (s *Store) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	var objectKey string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(object_key,'') FROM kb_documents
		WHERE id = $1 AND account_id = $2`, id, accountID).Scan(&objectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM kb_chunks WHERE document_id = $1 AND account_id = $2`, id, accountID)
	if err != nil {
		return err
	}
	var chunkIDs []uuid.UUID
	for rows.Next() {
		var cid uuid.UUID
		if err := rows.Scan(&cid); err != nil {
			rows.Close()
			return err
		}
		chunkIDs = append(chunkIDs, cid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, cid := range chunkIDs {
		s.unproject(ctx, accountID, cid)
	}
	if objectKey != "" {
		if err := s.objects.Delete(ctx, objectKey); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
			log.Printf("kb object delete %s: %v", objectKey, err)
		}
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM kb_documents WHERE id = $1 AND account_id = $2`, id, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Reparse(ctx context.Context, accountID, id uuid.UUID) (Document, error) {
	doc, err := scanDoc(s.pool.QueryRow(ctx, `
		UPDATE kb_documents
		SET status = $1, error = '', updated_at = now()
		WHERE id = $2 AND account_id = $3
		RETURNING `+docColumns, StatusPending, id, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	if err := s.enqueue.Enqueue(ctx, queue.Job{
		Type: queue.JobKBParse, AccountID: accountID, DocumentID: doc.ID,
	}); err != nil {
		log.Printf("enqueue kb_parse reparse %s: %v", doc.ID, err)
	}
	return doc, nil
}

func (s *Store) GetManyChunks(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID) ([]Chunk, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, document_id, ordinal, COALESCE(body,''), char_start, char_end
		FROM kb_chunks
		WHERE account_id = $1 AND id = ANY($2)
		ORDER BY document_id, ordinal`, accountID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Ordinal, &c.Body, &c.CharStart, &c.CharEnd); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SetStatus(ctx context.Context, accountID, docID uuid.UUID, status, errMsg string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE kb_documents
		SET status = $1, error = $2, updated_at = now()
		WHERE id = $3 AND account_id = $4`, status, errMsg, docID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceChunks deletes prior chunks (and index entries), inserts the new set,
// indexes each as DocKBChunk with Kind=document title, and marks the doc ready.
func (s *Store) ReplaceChunks(ctx context.Context, accountID, docID uuid.UUID, chunks []Chunk) error {
	doc, err := s.Get(ctx, accountID, docID)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM kb_chunks WHERE document_id = $1 AND account_id = $2`, docID, accountID)
	if err != nil {
		return err
	}
	var oldIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		oldIDs = append(oldIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM kb_chunks WHERE document_id = $1 AND account_id = $2`, docID, accountID); err != nil {
		return err
	}

	saved := make([]Chunk, 0, len(chunks))
	for i, c := range chunks {
		id := c.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		ord := c.Ordinal
		if ord == 0 && i > 0 {
			ord = i
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO kb_chunks (id, document_id, account_id, ordinal, body, char_start, char_end)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			id, docID, accountID, ord, c.Body, c.CharStart, c.CharEnd)
		if err != nil {
			return err
		}
		saved = append(saved, Chunk{
			ID: id, DocumentID: docID, Ordinal: ord, Body: c.Body,
			CharStart: c.CharStart, CharEnd: c.CharEnd,
		})
	}
	if _, err := tx.Exec(ctx, `
		UPDATE kb_documents
		SET status = $1, error = '', updated_at = now()
		WHERE id = $2 AND account_id = $3`, StatusReady, docID, accountID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	for _, id := range oldIDs {
		s.unproject(ctx, accountID, id)
	}
	now := time.Now().UTC()
	for _, c := range saved {
		s.projectChunk(ctx, accountID, doc.Title, c, now)
	}
	return nil
}

func (s *Store) projectChunk(ctx context.Context, accountID uuid.UUID, title string, c Chunk, at time.Time) {
	if s.index == nil {
		return
	}
	if err := s.index.IndexDoc(ctx, index.Doc{
		ID:        c.ID,
		DocType:   index.DocKBChunk,
		AccountID: accountID,
		ProjectID: nil,
		Kind:      title,
		Body:      c.Body,
		CreatedAt: at,
	}); err != nil {
		log.Printf("kb index chunk %s: %v", c.ID, err)
	}
}

func (s *Store) unproject(ctx context.Context, accountID, chunkID uuid.UUID) {
	if s.index == nil {
		return
	}
	if err := s.index.Delete(ctx, accountID, chunkID); err != nil {
		log.Printf("kb unindex chunk %s: %v", chunkID, err)
	}
}
