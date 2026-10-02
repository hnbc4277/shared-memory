package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	VaultAccept     = "vault.accept"
	VaultReject     = "vault.reject"
	VaultCreate     = "vault.create"
	VaultReveal     = "vault.reveal"
	VaultCopy       = "vault.copy"
	VaultUpdate     = "vault.update"
	VaultDelete     = "vault.delete"
	ArtifactAccept  = "artifact.accept"
	ArtifactReject  = "artifact.reject"
	ArtifactCreate  = "artifact.create"
	ArtifactDelete  = "artifact.delete"
	PolicyUpdate    = "policy.update"
	ProjectDelete   = "project.delete"
	APIKeyCreate    = "apikey.create"
	APIKeyRevoke    = "apikey.revoke"
	MemorySupersede = "memory.supersede"
	MemoryInvalid   = "memory.invalidate"
)

type Entry struct {
	ID        uuid.UUID       `json:"id"`
	Action    string          `json:"action"`
	Meta      json.RawMessage `json:"meta"`
	CreatedAt time.Time       `json:"created_at"`
}

type Log struct {
	pool *pgxpool.Pool
}

func NewLog(pool *pgxpool.Pool) *Log {
	return &Log{pool: pool}
}

func (l *Log) Record(ctx context.Context, accountID uuid.UUID, action string, meta map[string]any) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = l.pool.Exec(ctx,
		`INSERT INTO audit_logs (account_id, action, meta) VALUES ($1, $2, $3)`,
		accountID, action, b,
	)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return nil
}

func (l *Log) List(ctx context.Context, accountID uuid.UUID, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := l.pool.Query(ctx,
		`SELECT id, action, COALESCE(meta, '{}'::jsonb), created_at FROM audit_logs
		 WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2`,
		accountID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Action, &e.Meta, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
