package quarantine

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store keeps rejected ingest attempts out of formal memory (design §层1).
type Store struct {
	Pool *pgxpool.Pool
}

type Entry struct {
	ID        uuid.UUID       `json:"id"`
	AccountID *uuid.UUID      `json:"account_id,omitempty"`
	Agent     string          `json:"agent"`
	AgentPath string          `json:"agent_path"`
	Reason    string          `json:"reason"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *Store) Add(ctx context.Context, accountID *uuid.UUID, agent, agentPath, reason string, payload any) error {
	if s == nil || s.Pool == nil {
		return nil
	}
	raw, _ := json.Marshal(payload)
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO ingest_quarantine (account_id, agent, agent_path, reason, payload)
		VALUES ($1,$2,$3,$4,$5)`, accountID, agent, agentPath, reason, raw)
	return err
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, account_id, COALESCE(agent,''), COALESCE(agent_path,''), reason, payload, created_at
		FROM ingest_quarantine WHERE account_id = $1
		ORDER BY created_at DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.AccountID, &e.Agent, &e.AgentPath, &e.Reason, &e.Payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
