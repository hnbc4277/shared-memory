package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ListFilter struct {
	ProjectID    *uuid.UUID
	ConnectionID *uuid.UUID
	Agent        string
	// AgentPath matches the path itself and all of its sub-agents.
	AgentPath string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

const sessionColumns = `s.id, s.project_id, s.agent, s.agent_path, COALESCE(s.title,''), COALESCE(s.status,''),
	COALESCE(s.started_at, now()), COALESCE(s.ended_at, s.started_at, now()), s.degraded,
	(SELECT count(*) FROM messages m WHERE m.session_id = s.id),
	COALESCE((SELECT jsonb_agg(DISTINCT m.agent_path) FROM messages m WHERE m.session_id = s.id), '[]'::jsonb)`

func scanSession(row pgx.Row) (Session, error) {
	var se Session
	var paths []byte
	err := row.Scan(&se.ID, &se.ProjectID, &se.Agent, &se.AgentPath, &se.Title, &se.Status,
		&se.StartedAt, &se.LastActiveAt, &se.Degraded, &se.MessageCount, &paths)
	if err != nil {
		return Session{}, err
	}
	se.AgentPaths = []string{}
	_ = json.Unmarshal(paths, &se.AgentPaths)
	return se, nil
}

func likePrefix(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s) + "/%"
}

func (s *Service) List(ctx context.Context, accountID uuid.UUID, f ListFilter) ([]Session, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+sessionColumns+` FROM sessions s
		WHERE s.account_id = $1
		  AND ($2::uuid IS NULL OR s.project_id = $2)
		  AND ($3::uuid IS NULL OR s.connection_id = $3)
		  AND ($4 = '' OR s.agent = $4)
		  AND ($5 = '' OR EXISTS (SELECT 1 FROM messages m WHERE m.session_id = s.id
		                          AND (m.agent_path = $5 OR m.agent_path LIKE $6)))
		  AND ($7::timestamptz IS NULL OR COALESCE(s.ended_at, s.started_at) >= $7)
		  AND ($8::timestamptz IS NULL OR s.started_at <= $8)
		ORDER BY COALESCE(s.ended_at, s.started_at) DESC
		LIMIT $9 OFFSET $10`,
		accountID, f.ProjectID, f.ConnectionID, f.Agent, f.AgentPath, likePrefix(f.AgentPath), f.From, f.To, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		se, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

// Get returns a session with its messages. Content is read back from the
// transcript object; when transcript storage was off only provenance remains.
func (s *Service) Get(ctx context.Context, accountID, sessionID uuid.UUID) (SessionDetail, error) {
	se, err := scanSession(s.Pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM sessions s
		WHERE s.id = $1 AND s.account_id = $2`, sessionID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionDetail{}, fmt.Errorf("%w: session", ErrNotFound)
	}
	if err != nil {
		return SessionDetail{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id, m.seq, COALESCE(m.role,''), m.agent, m.agent_path, m.skills, m.model, m.content_object_key,
		       m.attachment_ids, m.created_at,
		       COALESCE(e.plugins, '[]'::jsonb), COALESCE(e.tools, '[]'::jsonb)
		FROM messages m
		LEFT JOIN events e ON e.message_id = m.id AND e.kind = 'message'
		WHERE m.session_id = $1 AND m.account_id = $2
		ORDER BY m.seq`, sessionID, accountID)
	if err != nil {
		return SessionDetail{}, err
	}
	type pending struct {
		msg Message
		key *string
	}
	var list []pending
	for rows.Next() {
		var p pending
		var skills, atts, plugins, tools []byte
		if err := rows.Scan(&p.msg.ID, &p.msg.Seq, &p.msg.Role, &p.msg.Agent, &p.msg.AgentPath, &skills,
			&p.msg.Model, &p.key, &atts, &p.msg.CreatedAt, &plugins, &tools); err != nil {
			rows.Close()
			return SessionDetail{}, err
		}
		p.msg.Skills, p.msg.Plugins, p.msg.Tools, p.msg.AttachmentIDs = []string{}, []string{}, []string{}, []string{}
		_ = json.Unmarshal(skills, &p.msg.Skills)
		_ = json.Unmarshal(plugins, &p.msg.Plugins)
		_ = json.Unmarshal(tools, &p.msg.Tools)
		_ = json.Unmarshal(atts, &p.msg.AttachmentIDs)
		list = append(list, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return SessionDetail{}, err
	}
	callsByMsg, err := s.loadToolCalls(ctx, accountID, sessionID)
	if err != nil {
		return SessionDetail{}, err
	}
	d := SessionDetail{Session: se, Messages: make([]Message, 0, len(list))}
	for _, p := range list {
		if p.key != nil {
			if text, err := s.readTranscript(ctx, *p.key); err == nil {
				p.msg.Content = &text
				p.msg.ContentStored = true
			}
		}
		p.msg.ToolCalls = callsByMsg[p.msg.ID]
		d.Messages = append(d.Messages, p.msg)
	}
	return d, nil
}

func (s *Service) loadToolCalls(ctx context.Context, accountID, sessionID uuid.UUID) (map[uuid.UUID][]ToolCall, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT message_id, kind, name, started_at, ended_at, COALESCE(output_truncated, '')
		FROM tool_calls WHERE session_id = $1 AND account_id = $2
		ORDER BY started_at`, sessionID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]ToolCall{}
	for rows.Next() {
		var msgID *uuid.UUID
		var tc ToolCall
		var started time.Time
		var ended *time.Time
		if err := rows.Scan(&msgID, &tc.Kind, &tc.Name, &started, &ended, &tc.Output); err != nil {
			return nil, err
		}
		tc.StartedAt = started.UTC().Format(time.RFC3339)
		if ended != nil {
			tc.EndedAt = ended.UTC().Format(time.RFC3339)
		}
		if msgID == nil {
			continue
		}
		out[*msgID] = append(out[*msgID], tc)
	}
	return out, rows.Err()
}

type MessageRef struct {
	SessionID uuid.UUID
	ProjectID *uuid.UUID
	AgentPath string
	Content   *string
}

// MessageContent resolves one message; Content is nil when the transcript was not stored.
func (s *Service) MessageContent(ctx context.Context, accountID, messageID uuid.UUID) (MessageRef, error) {
	var ref MessageRef
	var key *string
	err := s.Pool.QueryRow(ctx, `SELECT m.session_id, s.project_id, m.agent_path, m.content_object_key
		FROM messages m JOIN sessions s ON s.id = m.session_id
		WHERE m.id = $1 AND m.account_id = $2`, messageID, accountID).Scan(&ref.SessionID, &ref.ProjectID, &ref.AgentPath, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return MessageRef{}, fmt.Errorf("%w: message", ErrNotFound)
	}
	if err != nil {
		return MessageRef{}, err
	}
	if key != nil {
		text, err := s.readTranscript(ctx, *key)
		if err != nil {
			return MessageRef{}, err
		}
		ref.Content = &text
	}
	return ref, nil
}

func (s *Service) readTranscript(ctx context.Context, key string) (string, error) {
	rc, err := s.Objects.Get(ctx, key)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, 8<<20))
	if err != nil {
		return "", err
	}
	var e TranscriptEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return "", err
	}
	return e.Content, nil
}
