package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/attachment"
	"sharedmemory/internal/entity"
	"sharedmemory/internal/memory"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/vault"
)

const (
	episodeLines   = 12
	runningLines   = 20
	maxDerivedText = 20000
	attachmentHead = 1000
)

// Extractor turns an attachment binary into searchable text.
type Extractor interface {
	Extract(ctx context.Context, mimeType string, data []byte) (string, error)
}

// TextExtractor handles text attachments only; tests use it to avoid tools.
type TextExtractor struct{}

func (TextExtractor) Extract(_ context.Context, mimeType string, data []byte) (string, error) {
	if !attachment.IsText(mimeType) || !utf8.Valid(data) {
		return "", nil
	}
	return string(data), nil
}

type Handler struct {
	Pool      *pgxpool.Pool
	Objects   objectstore.Store
	Memories  *memory.Store
	Deriver   semantic.Deriver
	Extractor Extractor
	Entities  *entity.Store
	Policies  *policy.Store
}

func (h *Handler) Handle(ctx context.Context, job queue.Job) error {
	switch job.Type {
	case queue.JobMessage:
		return h.handleMessage(ctx, job)
	case queue.JobAttachment:
		return h.handleAttachment(ctx, job)
	default:
		return fmt.Errorf("unknown job type %q", job.Type)
	}
}

// appendLine keeps the newest n lines of a rolling summary.
func appendLine(old, line string, n int) string {
	lines := []string{}
	for _, l := range strings.Split(old, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	lines = append(lines, line)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (h *Handler) sessionExists(ctx context.Context, accountID, sessionID uuid.UUID) (bool, error) {
	var ok bool
	err := h.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sessions WHERE id = $1 AND account_id = $2)`,
		sessionID, accountID).Scan(&ok)
	return ok, err
}

func (h *Handler) handleMessage(ctx context.Context, job queue.Job) error {
	if ok, err := h.sessionExists(ctx, job.AccountID, job.SessionID); err != nil || !ok {
		// The session was deleted before processing; nothing left to derive.
		return err
	}
	content := vault.Redact(job.Content)
	if strings.TrimSpace(content) == "" {
		return nil
	}
	sessionID := job.SessionID
	summary, err := h.Deriver.Summarize(ctx, content)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("[%s·%s] %s", job.Role, job.AgentPath, summary)
	if _, err := h.Memories.UpsertRolling(ctx, job.AccountID, memory.Memory{
		ProjectID: job.ProjectID, Scope: semantic.ScopeProject, Kind: semantic.KindEpisodeSummary,
		SourceSessionID: &sessionID, SourceAgentPath: job.AgentPath,
	}, func(old string) string { return appendLine(old, line, episodeLines) }); err != nil {
		return fmt.Errorf("episode summary: %w", err)
	}
	if job.ProjectID != nil {
		if _, err := h.Memories.UpsertRolling(ctx, job.AccountID, memory.Memory{
			ProjectID: job.ProjectID, Scope: semantic.ScopeProject, Kind: semantic.KindRunningSummary,
			SourceSessionID: &sessionID, SourceAgentPath: job.AgentPath,
		}, func(old string) string { return appendLine(old, line, runningLines) }); err != nil {
			return fmt.Errorf("running summary: %w", err)
		}
	}
	extracted, err := h.Deriver.Extract(ctx, job.Role, content, job.ProjectID != nil)
	if err != nil {
		return err
	}
	profileDirty := false
	for _, e := range extracted {
		m := memory.Memory{ProjectID: job.ProjectID, Scope: e.Scope, Kind: e.Kind, Body: e.Body,
			SourceSessionID: &sessionID, SourceAgentPath: job.AgentPath}
		saved, created, err := h.Memories.Add(ctx, job.AccountID, m)
		if err != nil {
			return fmt.Errorf("add %s: %w", e.Kind, err)
		}
		if created && e.Kind == semantic.KindPreference {
			profileDirty = true
		}
		if created && e.Kind == semantic.KindEntity && h.Entities != nil {
			if err := h.Entities.RecordFromBody(ctx, job.AccountID, job.ProjectID, saved.ID, e.Body); err != nil {
				return fmt.Errorf("entity alias: %w", err)
			}
		}
	}
	if profileDirty {
		if _, err := h.Memories.RebuildProfile(ctx, job.AccountID); err != nil {
			return fmt.Errorf("profile: %w", err)
		}
	}
	return nil
}

func (h *Handler) handleAttachment(ctx context.Context, job queue.Job) error {
	var (
		key            *string
		mimeType, name string
		projectID      *uuid.UUID
		sessionID      *uuid.UUID
		agentPath      string
	)
	err := h.Pool.QueryRow(ctx, `SELECT object_key, COALESCE(mime,''), COALESCE(filename,''), project_id, session_id,
		COALESCE(agent_path,'') FROM attachments WHERE id = $1 AND account_id = $2`,
		job.AttachmentID, job.AccountID).Scan(&key, &mimeType, &name, &projectID, &sessionID, &agentPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if key == nil {
		return nil
	}
	rc, err := h.Objects.Get(ctx, *key)
	if errors.Is(err, objectstore.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(rc, attachment.MaxBytes))
	rc.Close()
	if err != nil {
		return err
	}
	allowVision := false
	if h.Policies != nil {
		if pol, err := h.Policies.Get(ctx, job.AccountID); err == nil {
			allowVision = pol.CloudVisionEnabled
		}
	}
	text, err := h.Extractor.Extract(WithCloudVision(ctx, allowVision), mimeType, data)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(vault.Redact(text))
	if text == "" {
		return nil
	}
	text = semantic.Truncate(text, maxDerivedText)
	if _, err := h.Pool.Exec(ctx, `UPDATE attachments SET derived_text = $3 WHERE id = $1 AND account_id = $2`,
		job.AttachmentID, job.AccountID, text); err != nil {
		return err
	}
	_, _, err = h.Memories.Add(ctx, job.AccountID, memory.Memory{
		ProjectID: projectID, Scope: semantic.ScopeProject, Kind: semantic.KindAttachmentText,
		Body:            fmt.Sprintf("附件 %s：%s", name, semantic.Truncate(semantic.Collapse(text), attachmentHead)),
		SourceSessionID: sessionID, SourceAgentPath: agentPath,
	})
	return err
}
