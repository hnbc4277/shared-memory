package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const defaultQueryPreviewRunes = 200

type Receipt struct {
	ID                uuid.UUID  `json:"id"`
	ProjectID         *uuid.UUID `json:"project_id,omitempty"`
	SessionID         *uuid.UUID `json:"session_id,omitempty"`
	ExternalSessionID string     `json:"external_session_id,omitempty"`
	Agent             string     `json:"agent,omitempty"`
	AgentPath         string     `json:"agent_path,omitempty"`
	QueriedAt         time.Time  `json:"queried_at"`
	HitCount          int        `json:"hit_count"`
	KBHitCount        int        `json:"kb_hit_count"`
	Degraded          bool       `json:"degraded"`
	Warnings          []string   `json:"warnings"`
	Source            string     `json:"source"`
	QueryPreview      string     `json:"query_preview,omitempty"`
}

type ReceiptFilter struct {
	SessionID         *uuid.UUID
	ExternalSessionID string
	Limit             int
}

func previewQuery(q string, maxRunes int) string {
	q = strings.TrimSpace(q)
	if maxRunes <= 0 || utf8.RuneCountInString(q) <= maxRunes {
		return q
	}
	return string([]rune(q)[:maxRunes])
}

func isDegraded(warnings []string) bool {
	for _, w := range warnings {
		if w == WarnIndexDegraded || w == WarnPGFallback {
			return true
		}
	}
	return false
}

func buildReceipt(req Request, res Result) Receipt {
	warnings := res.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "http"
	}
	return Receipt{
		ProjectID:         req.ProjectID,
		SessionID:         req.SessionID,
		ExternalSessionID: strings.TrimSpace(req.ExternalSessionID),
		Agent:             strings.TrimSpace(req.Agent),
		AgentPath:         strings.TrimSpace(req.AgentPath),
		HitCount:          len(res.Hits),
		KBHitCount:        len(res.KBHits),
		Degraded:          isDegraded(warnings),
		Warnings:          warnings,
		Source:            source,
		QueryPreview:      previewQuery(req.Query, defaultQueryPreviewRunes),
	}
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Service) recordReceipt(ctx context.Context, accountID uuid.UUID, req Request, res Result) (uuid.UUID, error) {
	if s.Pool == nil {
		return uuid.Nil, fmt.Errorf("recall: pool required for receipts")
	}
	r := buildReceipt(req, res)
	warnJSON, err := json.Marshal(r.Warnings)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO recall_receipts (
			account_id, project_id, session_id, external_session_id,
			agent, agent_path, hit_count, kb_hit_count, degraded, warnings, source, query_preview
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id`,
		accountID, r.ProjectID, r.SessionID, nullStr(r.ExternalSessionID),
		nullStr(r.Agent), nullStr(r.AgentPath), r.HitCount, r.KBHitCount, r.Degraded, warnJSON, r.Source, nullStr(r.QueryPreview),
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("recall receipt: %w", err)
	}
	return id, nil
}

func (s *Service) ListReceipts(ctx context.Context, accountID uuid.UUID, filter ReceiptFilter) ([]Receipt, error) {
	if s.Pool == nil {
		return nil, fmt.Errorf("recall: pool required for receipts")
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := `SELECT id, project_id, session_id, COALESCE(external_session_id, ''),
		COALESCE(agent, ''), COALESCE(agent_path, ''), queried_at, hit_count, kb_hit_count, degraded,
		COALESCE(warnings, '[]'::jsonb), source, COALESCE(query_preview, '')
		FROM recall_receipts WHERE account_id = $1`
	args := []any{accountID}
	n := 2
	if filter.SessionID != nil {
		q += fmt.Sprintf(` AND session_id = $%d`, n)
		args = append(args, *filter.SessionID)
		n++
	}
	if ext := strings.TrimSpace(filter.ExternalSessionID); ext != "" {
		q += fmt.Sprintf(` AND external_session_id = $%d`, n)
		args = append(args, ext)
		n++
	}
	q += fmt.Sprintf(` ORDER BY queried_at DESC LIMIT $%d`, n)
	args = append(args, limit)

	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Receipt{}
	for rows.Next() {
		var r Receipt
		var warnJSON []byte
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.SessionID, &r.ExternalSessionID,
			&r.Agent, &r.AgentPath, &r.QueriedAt, &r.HitCount, &r.KBHitCount, &r.Degraded,
			&warnJSON, &r.Source, &r.QueryPreview); err != nil {
			return nil, err
		}
		r.Warnings = []string{}
		_ = json.Unmarshal(warnJSON, &r.Warnings)
		out = append(out, r)
	}
	return out, rows.Err()
}
