package entity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound  = errors.New("entity alias not found")
	ErrConflict  = errors.New("entity alias already exists for this project")
)

// Store maps entity aliases to a canonical name (design §层2 Entity).
type Store struct {
	Pool *pgxpool.Pool
}

type Alias struct {
	ID         uuid.UUID  `json:"id"`
	ProjectID  *uuid.UUID `json:"project_id"`
	Canonical  string     `json:"canonical"`
	Alias      string     `json:"alias"`
	SourceMemoryID *uuid.UUID `json:"source_memory_id,omitempty"`
}

func (s *Store) Upsert(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID, canonical, alias string, memoryID *uuid.UUID) error {
	canonical = strings.TrimSpace(canonical)
	alias = strings.TrimSpace(alias)
	if canonical == "" || alias == "" {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `
		DELETE FROM entity_aliases
		WHERE account_id = $1 AND alias = $2 AND project_id IS NOT DISTINCT FROM $3`,
		accountID, alias, projectID)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO entity_aliases (account_id, project_id, canonical, alias, source_memory_id)
		VALUES ($1,$2,$3,$4,$5)`,
		accountID, projectID, canonical, alias, memoryID)
	return err
}

// RecordFromBody extracts "X（又名Y）" / "X(aka Y)" style aliases from entity memory bodies.
func (s *Store) RecordFromBody(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID, memoryID uuid.UUID, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	// Patterns: "Foo（又名Bar）" or "Foo (aka Bar)" or "Foo/Bar"
	canonical, alias := splitAlias(body)
	if alias == "" {
		return s.Upsert(ctx, accountID, projectID, body, body, &memoryID)
	}
	if err := s.Upsert(ctx, accountID, projectID, canonical, canonical, &memoryID); err != nil {
		return err
	}
	return s.Upsert(ctx, accountID, projectID, canonical, alias, &memoryID)
}

// SplitAlias exposes alias parsing for tests and recall expansion.
func SplitAlias(body string) (canonical, alias string) {
	return splitAlias(body)
}

func splitAlias(body string) (canonical, alias string) {
	for _, sep := range []string{"（又名", "(aka ", "(AKA ", "（又叫"} {
		if i := strings.Index(body, sep); i > 0 {
			canonical = strings.TrimSpace(body[:i])
			rest := body[i+len(sep):]
			rest = strings.TrimSuffix(strings.TrimSuffix(rest, "）"), ")")
			return canonical, strings.TrimSpace(rest)
		}
	}
	if i := strings.Index(body, "/"); i > 0 && !strings.Contains(body[:i], "http") {
		return strings.TrimSpace(body[:i]), strings.TrimSpace(body[i+1:])
	}
	return body, ""
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID) ([]Alias, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, project_id, canonical, alias, source_memory_id
		FROM entity_aliases
		WHERE account_id = $1 AND ($2::uuid IS NULL OR project_id IS NOT DISTINCT FROM $2)
		ORDER BY canonical, alias`, accountID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Canonical, &a.Alias, &a.SourceMemoryID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Update(ctx context.Context, accountID, id uuid.UUID, canonical, alias string) error {
	canonical = strings.TrimSpace(canonical)
	alias = strings.TrimSpace(alias)
	if canonical == "" || alias == "" {
		return fmt.Errorf("canonical and alias are required")
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE entity_aliases SET canonical = $3, alias = $4
		WHERE account_id = $1 AND id = $2`, accountID, id, canonical, alias)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrConflict
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM entity_aliases WHERE account_id = $1 AND id = $2`, accountID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Resolve(ctx context.Context, accountID uuid.UUID, projectID *uuid.UUID, name string) (string, error) {
	name = strings.TrimSpace(name)
	var canonical string
	err := s.Pool.QueryRow(ctx, `
		SELECT canonical FROM entity_aliases
		WHERE account_id = $1 AND alias = $2
		  AND ($3::uuid IS NULL OR project_id IS NOT DISTINCT FROM $3)
		LIMIT 1`, accountID, name, projectID).Scan(&canonical)
	if err != nil {
		return name, nil
	}
	return canonical, nil
}
