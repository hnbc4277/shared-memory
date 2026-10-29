package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/objectstore"
	"sharedmemory/internal/policy"
	"sharedmemory/internal/project"
	"sharedmemory/internal/queue"
	"sharedmemory/internal/vault"
)

var ErrNotFound = errors.New("not found")

const (
	WarnSecretRedacted   = "secret_redacted"
	WarnTranscriptFailed = "transcript_store_failed"
	WarnQueueUnavailable = "queue_unavailable"
	WarnVaultCandidate   = "vault_candidate_failed"
	WarnArtifactOffer    = "artifact_candidate_failed"
)

type Service struct {
	Pool      *pgxpool.Pool
	Objects   objectstore.Store
	Queue     queue.Enqueuer
	Policies  *policy.Store
	Projects  *project.Store
	Vault     *vault.Store
	Artifacts *artifact.Store
}

func optUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id := uuid.MustParse(s)
	return &id
}

// provenanceMark is the optional inline marker prepended to stored transcript
// text; the structured provenance columns are always written regardless.
func provenanceMark(req IngestMessageRequest) string {
	return fmt.Sprintf("[agent=%s skills=%s plugins=%s tools=%s model=%s]\n",
		req.AgentPath, strings.Join(*req.Skills, ","), strings.Join(*req.Plugins, ","),
		strings.Join(*req.Tools, ","), req.Model)
}

// Ingest stores one message with mandatory provenance. Secrets are redacted
// before anything is persisted or queued.
func (s *Service) Ingest(ctx context.Context, accountID uuid.UUID, req IngestMessageRequest) (IngestResult, error) {
	if err := ValidateIngest(req); err != nil {
		return IngestResult{}, err
	}
	req = Normalize(req)
	res := IngestResult{Warnings: []string{}, Prompts: []Prompt{}}

	projectID := optUUID(req.ProjectID)
	if projectID != nil {
		if _, err := s.Projects.Get(ctx, accountID, *projectID); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return IngestResult{}, fmt.Errorf("%w: project", ErrNotFound)
			}
			return IngestResult{}, err
		}
	}
	pol, err := s.Policies.Get(ctx, accountID)
	if err != nil {
		return IngestResult{}, err
	}

	var found []vault.Detected
	if pol.VaultEnabled {
		found = vault.Detect(req.Content)
	}
	content := vault.Redact(req.Content)
	if content != req.Content {
		res.Warnings = append(res.Warnings, WarnSecretRedacted)
	}

	attachmentIDs := make([]uuid.UUID, 0, len(req.AttachmentIDs))
	for _, id := range req.AttachmentIDs {
		attachmentIDs = append(attachmentIDs, uuid.MustParse(id))
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return IngestResult{}, err
	}
	defer tx.Rollback(ctx)

	if len(attachmentIDs) > 0 {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM attachments WHERE account_id = $1 AND id = ANY($2)`,
			accountID, attachmentIDs).Scan(&n); err != nil {
			return IngestResult{}, err
		}
		if n != len(attachmentIDs) {
			return IngestResult{}, fmt.Errorf("%w: attachment", ErrNotFound)
		}
	}

	var sessionID uuid.UUID
	if req.SessionID != "" {
		sessionID = uuid.MustParse(req.SessionID)
		var sessProject *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT project_id FROM sessions WHERE id = $1 AND account_id = $2 FOR UPDATE`,
			sessionID, accountID).Scan(&sessProject)
		if errors.Is(err, pgx.ErrNoRows) {
			return IngestResult{}, fmt.Errorf("%w: session", ErrNotFound)
		}
		if err != nil {
			return IngestResult{}, err
		}
		if projectID != nil && (sessProject == nil || *sessProject != *projectID) {
			return IngestResult{}, fmt.Errorf("%w: project_id does not match the session's project", ErrInvalid)
		}
		projectID = sessProject
	} else if req.ExternalSessionID != "" {
		err := tx.QueryRow(ctx, `
			SELECT id, project_id FROM sessions
			WHERE account_id = $1 AND agent = $2 AND external_session_id = $3
			FOR UPDATE`,
			accountID, req.Agent, req.ExternalSessionID,
		).Scan(&sessionID, &projectID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return IngestResult{}, err
		}
		if errors.Is(err, pgx.ErrNoRows) {
			sessionID = uuid.New()
			title := strings.TrimSpace(req.Title)
			if title == "" {
				title = firstLine(content, 60)
			}
			pid := optUUID(req.ProjectID)
			projectID = pid
			if _, err := tx.Exec(ctx, `
				INSERT INTO sessions (id, account_id, project_id, agent, agent_path, title, status, started_at, ended_at, external_session_id)
				VALUES ($1,$2,$3,$4,$5,$6,'active',now(),now(),$7)`,
				sessionID, accountID, projectID, req.Agent, req.AgentPath, title, req.ExternalSessionID); err != nil {
				return IngestResult{}, fmt.Errorf("insert session: %w", err)
			}
		} else if projectID != nil && req.ProjectID != "" {
			want := uuid.MustParse(req.ProjectID)
			if *projectID != want {
				return IngestResult{}, fmt.Errorf("%w: project_id does not match the session's project", ErrInvalid)
			}
		}
	} else {
		sessionID = uuid.New()
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = firstLine(content, 60)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO sessions (id, account_id, project_id, agent, agent_path, title, status, started_at, ended_at)
			VALUES ($1,$2,$3,$4,$5,$6,'active',now(),now())`,
			sessionID, accountID, projectID, req.Agent, req.AgentPath, title); err != nil {
			return IngestResult{}, fmt.Errorf("insert session: %w", err)
		}
	}

	var seq int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE session_id = $1`, sessionID).Scan(&seq); err != nil {
		return IngestResult{}, err
	}
	messageID := uuid.New()
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])
	attIDs := req.AttachmentIDs
	if attIDs == nil {
		attIDs = []string{}
	}

	var objectKey *string
	if pol.StoreTranscript {
		body := content
		if req.StoreTranscriptMark {
			body = provenanceMark(req) + content
		}
		entry, _ := json.Marshal(TranscriptEntry{
			MessageID: messageID, SessionID: sessionID, Seq: seq, Role: req.Role, Agent: req.Agent,
			AgentPath: req.AgentPath, Skills: *req.Skills, Plugins: *req.Plugins, Tools: *req.Tools,
			Model: req.Model, Content: body, AttachmentIDs: attIDs, CreatedAt: now,
		})
		key := objectstore.TranscriptKey(accountID, projectID, sessionID, seq, messageID)
		if err := s.Objects.Put(ctx, key, bytes.NewReader(entry), int64(len(entry)), "application/json"); err != nil {
			log.Printf("transcript put %s: %v", key, err)
			res.Warnings = append(res.Warnings, WarnTranscriptFailed)
		} else {
			objectKey = &key
		}
	}

	skillsJSON, _ := json.Marshal(*req.Skills)
	pluginsJSON, _ := json.Marshal(*req.Plugins)
	toolsJSON, _ := json.Marshal(*req.Tools)
	attJSON, _ := json.Marshal(attIDs)
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages (id, session_id, account_id, role, seq, content_object_key, skills, model,
			agent, agent_path, content_hash, attachment_ids, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		messageID, sessionID, accountID, req.Role, seq, objectKey, skillsJSON, req.Model,
		req.Agent, req.AgentPath, contentHash, attJSON, now); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, fmt.Errorf("insert message: %w", err)
	}
	payload, _ := json.Marshal(map[string]any{
		"store_transcript_mark": req.StoreTranscriptMark,
		"redacted":              content != req.Content,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO events (session_id, message_id, account_id, kind, plugins, tools, payload, created_at)
		VALUES ($1,$2,$3,'message',$4,$5,$6,$7)`,
		sessionID, messageID, accountID, pluginsJSON, toolsJSON, payload, now); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, fmt.Errorf("insert event: %w", err)
	}
	if err := insertToolCalls(ctx, tx, accountID, sessionID, messageID, req.ToolCalls); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, err
	}
	if len(attachmentIDs) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE attachments SET session_id = $3, message_id = $4,
			project_id = COALESCE(project_id, $5)
			WHERE account_id = $1 AND id = ANY($2)`,
			accountID, attachmentIDs, sessionID, messageID, projectID); err != nil {
			s.dropObject(objectKey)
			return IngestResult{}, err
		}
	}
	transcriptLost := pol.StoreTranscript && objectKey == nil
	if err := bindSessionConnection(ctx, tx, accountID, sessionID, req.ConnectionID); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE messages m SET connection_id = s.connection_id
		FROM sessions s
		WHERE m.id = $1 AND m.session_id = s.id AND s.connection_id IS NOT NULL
		  AND m.connection_id IS NULL`, messageID); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, fmt.Errorf("message connection_id: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET ended_at = $2, degraded = degraded OR $3 WHERE id = $1`,
		sessionID, now, transcriptLost); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		s.dropObject(objectKey)
		return IngestResult{}, err
	}
	res.SessionID, res.MessageID, res.Seq = sessionID, messageID, seq

	if len(found) > 0 && s.Vault != nil {
		cands, err := s.Vault.CreateCandidates(ctx, accountID,
			vault.Source{ProjectID: projectID, SessionID: sessionID, MessageID: messageID, AgentPath: req.AgentPath}, found)
		if err != nil {
			log.Printf("vault candidates: %v", err)
			res.Warnings = append(res.Warnings, WarnVaultCandidate)
		}
		for _, c := range cands {
			res.Prompts = append(res.Prompts, Prompt{Kind: "vault", CandidateID: c.ID.String(),
				Text: fmt.Sprintf("检测到「%s」的账号/口令，是否加密保存到凭证库？", c.Title)})
		}
	}
	if s.Artifacts != nil && artifact.ShouldOffer(req.Role, content, req.ArtifactOffer) {
		c, err := s.Artifacts.CreateCandidate(ctx, accountID,
			artifact.Source{ProjectID: projectID, SessionID: &sessionID, MessageID: &messageID, AgentPath: req.AgentPath}, content)
		if err != nil {
			log.Printf("artifact candidate: %v", err)
			res.Warnings = append(res.Warnings, WarnArtifactOffer)
		} else {
			res.Prompts = append(res.Prompts, Prompt{Kind: "artifact", CandidateID: c.ID.String(),
				Text: fmt.Sprintf("AI 生成了「%s」，是否保存到成稿库方便以后查看？", c.Title)})
		}
	}

	job := queue.Job{Type: queue.JobMessage, AccountID: accountID, ProjectID: projectID, SessionID: sessionID,
		MessageID: messageID, Role: req.Role, AgentPath: req.AgentPath, Content: content}
	if err := s.Queue.Enqueue(ctx, job); err != nil {
		log.Printf("enqueue: %v", err)
		res.Warnings = append(res.Warnings, WarnQueueUnavailable)
		if _, err := s.Pool.Exec(ctx, `UPDATE sessions SET degraded = TRUE WHERE id = $1`, sessionID); err != nil {
			log.Printf("mark degraded: %v", err)
		}
	}
	return res, nil
}

func (s *Service) dropObject(key *string) {
	if key == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Objects.Delete(ctx, *key)
}

func firstLine(s string, n int) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			r := []rune(line)
			if len(r) > n {
				return string(r[:n]) + "…"
			}
			return line
		}
	}
	return "未命名会话"
}

const maxToolOutputRunes = 4000

func insertToolCalls(ctx context.Context, tx pgx.Tx, accountID, sessionID, messageID uuid.UUID, calls []ToolCall) error {
	for _, c := range calls {
		kind := strings.TrimSpace(c.Kind)
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		if kind != "plugin" && kind != "tool" {
			kind = "tool"
		}
		out := c.Output
		if r := []rune(out); len(r) > maxToolOutputRunes {
			out = string(r[:maxToolOutputRunes]) + "…"
		}
		started := time.Now().UTC()
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(c.StartedAt)); err == nil {
			started = t.UTC()
		}
		var ended *time.Time
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(c.EndedAt)); err == nil {
			u := t.UTC()
			ended = &u
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tool_calls (account_id, session_id, message_id, kind, name, started_at, ended_at, output_truncated)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			accountID, sessionID, messageID, kind, name, started, ended, out); err != nil {
			return fmt.Errorf("insert tool_call: %w", err)
		}
	}
	return nil
}

func bindSessionConnection(ctx context.Context, tx pgx.Tx, accountID, sessionID uuid.UUID, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	cid, err := uuid.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: invalid connection_id", ErrInvalid)
	}
	_, err = tx.Exec(ctx, `
		UPDATE sessions SET connection_id = $1
		WHERE id = $2 AND account_id = $3 AND (connection_id IS NULL OR connection_id = $1)`,
		cid, sessionID, accountID)
	return err
}
