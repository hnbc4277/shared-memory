package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"sharedmemory/internal/kb"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/queue"
)

func (h *Handler) handleKBParse(ctx context.Context, job queue.Job) error {
	if h.KB == nil {
		return fmt.Errorf("kb store not configured")
	}
	doc, err := h.KB.Get(ctx, job.AccountID, job.DocumentID)
	if errors.Is(err, kb.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	key := kb.ObjectKey(job.AccountID, doc.ID)
	rc, err := h.Objects.Get(ctx, key)
	if errors.Is(err, objectstore.ErrNotFound) {
		return h.KB.SetStatus(ctx, job.AccountID, doc.ID, kb.StatusFailed, "object missing")
	}
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(rc, kb.MaxBytes))
	rc.Close()
	if err != nil {
		return err
	}

	text, err := h.Extractor.Extract(ctx, doc.MIME, data)
	if err != nil {
		_ = h.KB.SetStatus(ctx, job.AccountID, doc.ID, kb.StatusFailed, err.Error())
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return h.KB.SetStatus(ctx, job.AccountID, doc.ID, kb.StatusFailed, "no text extracted")
	}

	parts := kb.ChunkText(text, kb.DefaultMaxRunes)
	chunks := make([]kb.Chunk, len(parts))
	for i, p := range parts {
		chunks[i] = kb.Chunk{Ordinal: i, Body: p.Body, CharStart: p.CharStart, CharEnd: p.CharEnd}
	}
	return h.KB.ReplaceChunks(ctx, job.AccountID, doc.ID, chunks)
}
