package memory

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/index"
	"sharedmemory/internal/semantic"
)

var ErrNotFound = errors.New("memory not found")

type Memory struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       *uuid.UUID `json:"project_id"`
	Scope           string     `json:"scope"`
	Kind            string     `json:"kind"`
	Body            string     `json:"body"`
	SourceSessionID *uuid.UUID `json:"source_session_id"`
	SourceAgentPath string     `json:"source_agent_path"`
	Supersedes      *uuid.UUID `json:"supersedes"`
	Invalidated     bool       `json:"invalidated"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Filter struct {
	ProjectID          *uuid.UUID
	Kind               string
	Scope              string
	IncludeInvalidated bool
	Limit              int
	Offset             int
}

// Store owns L2 semantic rows and keeps the L3 projection in sync. Index
// failures are logged, never fatal: recall falls back to Postgres.
type Store struct {
	pool  *pgxpool.Pool
	index *index.Service
}

func NewStore(pool *pgxpool.Pool, idx *index.Service) *Store {
	return &Store{pool: pool, index: idx}
}

const columns = `id, project_id, scope, COALESCE(kind,''), COALESCE(body,''), source_session_id,
	COALESCE(source_agent_path,''), supersedes, invalidated, created_at, updated_at`

func scan(row pgx.Row) (Memory, error) {
	var m Memory
	err := row.Scan(&m.ID, &m.ProjectID, &m.Scope, &m.Kind, &m.Body, &m.SourceSessionID,
		&m.SourceAgentPath, &m.Supersedes, &m.Invalidated, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (s *Store) project(ctx context.Context, accountID uuid.UUID, m Memory) {
	if s.index == nil {
		return
	}
	err := s.index.IndexDoc(ctx, index.Doc{
		ID: m.ID, DocType: index.DocMemory, AccountID: accountID, ProjectID: m.ProjectID, Scope: m.Scope,
		Kind: m.Kind, Body: m.Body, SourceSessionID: m.SourceSessionID, SourceAgentPath: m.SourceAgentPath,
		CreatedAt: m.CreatedAt,
	})
	if err != nil {
		log.Printf("memory index %s: %v", m.ID, err)
	}
}

func (s *Store) unproject(ctx context.Context, accountID, id uuid.UUID) {
	if s.index == nil {
		return
	}
	if err := s.index.Delete(ctx, accountID, id); err != nil {
		log.Printf("memory unindex %s: %v", id, err)
	}
}

func normalizeScope(m *Memory) {
	if m.ProjectID == nil {
		m.Scope = semantic.ScopeGlobal
	} else if m.Scope != semantic.ScopeGlobal {
		m.Scope = semantic.ScopeProject
	}
	if m.Scope == semantic.ScopeGlobal {
		m.ProjectID = nil
	}
}

// Add inserts a memory unless an identical live one already exists.
func (s *Store) Add(ctx context.Context, accountID uuid.UUID, m Memory) (Memory, bool, error) {
	normalizeScope(&m)
	m.Body = strings.TrimSpace(m.Body)
	if m.Body == "" {
		return Memory{}, false, fmt.Errorf("memory body is required")
	}
	existing, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM memories
		WHERE account_id = $1 AND project_id IS NOT DISTINCT FROM $2 AND kind = $3 AND body = $4 AND NOT invalidated
		LIMIT 1`, accountID, m.ProjectID, m.Kind, m.Body))
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Memory{}, false, err
	}
	m.ID = uuid.New()
	out, err := scan(s.pool.QueryRow(ctx, `
		INSERT INTO memories (id, account_id, project_id, scope, kind, body, source_session_id, source_agent_path, supersedes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+columns,
		m.ID, accountID, m.ProjectID, m.Scope, m.Kind, m.Body, m.SourceSessionID, m.SourceAgentPath, m.Supersedes))
	if err != nil {
		return Memory{}, false, fmt.Errorf("insert memory: %w", err)
	}
	s.project(ctx, accountID, out)
	return out, true, nil
}

// UpsertRolling keeps exactly one live row per (kind, session) or (kind, project)
// for rolling summaries, replacing its body.
func (s *Store) UpsertRolling(ctx context.Context, accountID uuid.UUID, m Memory, update func(old string) string) (Memory, error) {
	normalizeScope(&m)
	var row pgx.Row
	if m.SourceSessionID != nil && m.Kind == semantic.KindEpisodeSummary {
		row = s.pool.QueryRow(ctx, `SELECT `+columns+` FROM memories
			WHERE account_id = $1 AND kind = $2 AND source_session_id = $3 AND NOT invalidated LIMIT 1`,
			accountID, m.Kind, m.SourceSessionID)
	} else {
		row = s.pool.QueryRow(ctx, `SELECT `+columns+` FROM memories
			WHERE account_id = $1 AND kind = $2 AND project_id IS NOT DISTINCT FROM $3 AND NOT invalidated LIMIT 1`,
			accountID, m.Kind, m.ProjectID)
	}
	cur, err := scan(row)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		m.Body = update("")
		out, _, err := s.Add(ctx, accountID, m)
		return out, err
	case err != nil:
		return Memory{}, err
	}
	body := update(cur.Body)
	out, err := scan(s.pool.QueryRow(ctx, `
		UPDATE memories SET body = $3, source_agent_path = $4, updated_at = now()
		WHERE id = $1 AND account_id = $2 RETURNING `+columns,
		cur.ID, accountID, body, m.SourceAgentPath))
	if err != nil {
		return Memory{}, err
	}
	s.project(ctx, accountID, out)
	return out, nil
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID, f Filter) ([]Memory, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM memories
		WHERE account_id = $1
		  AND ($2::uuid IS NULL OR project_id = $2 OR project_id IS NULL)
		  AND ($3 = '' OR kind = $3)
		  AND ($4 = '' OR scope = $4)
		  AND ($5 OR NOT invalidated)
		ORDER BY updated_at DESC LIMIT $6 OFFSET $7`,
		accountID, f.ProjectID, f.Kind, f.Scope, f.IncludeInvalidated, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, accountID, id uuid.UUID) (Memory, error) {
	m, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM memories WHERE id = $1 AND account_id = $2`, id, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Memory{}, ErrNotFound
	}
	return m, err
}

// GetMany returns live memories by id, preserving the order of ids.
func (s *Store) GetMany(ctx context.Context, accountID uuid.UUID, ids []uuid.UUID) ([]Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM memories
		WHERE account_id = $1 AND id = ANY($2) AND NOT invalidated`, accountID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[uuid.UUID]Memory{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		byID[m.ID] = m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(byID))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// Supersede records a user correction: the old row is invalidated and a new
// row points back at it, so history stays auditable.
func (s *Store) Supersede(ctx context.Context, accountID, id uuid.UUID, body string) (Memory, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Memory{}, fmt.Errorf("body is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Memory{}, err
	}
	defer tx.Rollback(ctx)
	old, err := scan(tx.QueryRow(ctx, `UPDATE memories SET invalidated = TRUE, updated_at = now()
		WHERE id = $1 AND account_id = $2 AND NOT invalidated RETURNING `+columns, id, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Memory{}, ErrNotFound
	}
	if err != nil {
		return Memory{}, err
	}
	out, err := scan(tx.QueryRow(ctx, `
		INSERT INTO memories (id, account_id, project_id, scope, kind, body, source_session_id, source_agent_path, supersedes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+columns,
		uuid.New(), accountID, old.ProjectID, old.Scope, old.Kind, body, old.SourceSessionID, "user/correction", old.ID))
	if err != nil {
		return Memory{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Memory{}, err
	}
	s.unproject(ctx, accountID, old.ID)
	s.project(ctx, accountID, out)
	return out, nil
}

func (s *Store) Invalidate(ctx context.Context, accountID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `UPDATE memories SET invalidated = TRUE, updated_at = now()
		WHERE id = $1 AND account_id = $2 AND NOT invalidated`, id, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	s.unproject(ctx, accountID, id)
	return nil
}

// Search is the Postgres fallback used when both engines are unavailable.
func (s *Store) Search(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID, text string, limit int) ([]Memory, error) {
	terms := []string{}
	for _, t := range semantic.Tokens(text) {
		terms = append(terms, "%"+strings.NewReplacer("%", `\%`, "_", `\_`).Replace(t)+"%")
		if len(terms) >= 16 {
			break
		}
	}
	if len(terms) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM memories
		WHERE account_id = $1 AND NOT invalidated
		  AND (project_id IS NULL OR ($2::uuid IS NOT NULL AND project_id = $2))
		  AND body ILIKE ANY($3)
		ORDER BY updated_at DESC LIMIT $4`, accountID, projectID, terms, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Profile(ctx context.Context, accountID uuid.UUID) (string, time.Time, error) {
	var body string
	var at time.Time
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(body,''), updated_at FROM profiles WHERE account_id = $1`, accountID).Scan(&body, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, nil
	}
	return body, at, err
}

// RebuildProfile regenerates the global profile from live global preferences.
func (s *Store) RebuildProfile(ctx context.Context, accountID uuid.UUID) (string, error) {
	rows, err := s.pool.Query(ctx, `SELECT body FROM memories
		WHERE account_id = $1 AND scope = 'global' AND kind = $2 AND NOT invalidated
		ORDER BY updated_at DESC LIMIT 30`, accountID, semantic.KindPreference)
	if err != nil {
		return "", err
	}
	var lines []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return "", err
		}
		lines = append(lines, "- "+b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	body := strings.Join(lines, "\n")
	_, err = s.pool.Exec(ctx, `INSERT INTO profiles (account_id, body, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (account_id) DO UPDATE SET body = EXCLUDED.body, updated_at = now()`, accountID, body)
	return body, err
}
