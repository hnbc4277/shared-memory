package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("project not found")

type Project struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(ctx context.Context, accountID uuid.UUID, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	p := Project{ID: uuid.New(), Name: name}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO projects (id, account_id, name) VALUES ($1, $2, $3) RETURNING created_at`,
		p.ID, accountID, p.Name,
	).Scan(&p.CreatedAt)
	if err != nil {
		return Project{}, fmt.Errorf("insert project: %w", err)
	}
	return p, nil
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID) ([]Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, created_at FROM projects WHERE account_id = $1 ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns the project only when it belongs to accountID.
func (s *Store) Get(ctx context.Context, accountID, projectID uuid.UUID) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM projects WHERE account_id = $1 AND id = $2`,
		accountID, projectID,
	).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}
