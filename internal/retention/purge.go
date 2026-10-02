package retention

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/objectstore"
)

// batchSize bounds one purge transaction; Run loops until nothing is left.
const batchSize = 200

// Purger enforces account retention on the evidence layer: sessions whose
// last activity is older than retention_days lose their messages, events,
// attachments and transcripts. Derived memories, vault items and artifacts
// are kept; their pointer to the purged session is cleared.
type Purger struct {
	Pool    *pgxpool.Pool
	Objects objectstore.Store
}

type Stats struct {
	Sessions           int
	Attachments        int
	ArtifactCandidates int
}

type expired struct {
	id        uuid.UUID
	accountID uuid.UUID
	projectID *uuid.UUID
}

const expiredSessions = `
	SELECT s.id, s.account_id, s.project_id
	FROM sessions s JOIN account_policies p ON p.account_id = s.account_id
	WHERE p.retention_days > 0
	  AND COALESCE(s.ended_at, s.started_at) < now() - make_interval(days => p.retention_days)
	LIMIT $1`

// statements run with $1 = expired session ids, in FK-safe order.
var sessionStatements = []string{
	`DELETE FROM vault_candidates WHERE session_id = ANY($1)`,
	`UPDATE vault_items SET source_session_id = NULL WHERE source_session_id = ANY($1)`,
	`UPDATE artifacts SET source_session_id = NULL WHERE source_session_id = ANY($1)`,
	`UPDATE memories SET source_session_id = NULL WHERE source_session_id = ANY($1)`,
	`DELETE FROM events WHERE session_id = ANY($1)`,
	`DELETE FROM messages WHERE session_id = ANY($1)`,
	`DELETE FROM sessions WHERE id = ANY($1)`,
}

// Run purges every account's expired evidence and all expired artifact
// candidates. Object deletion failures are reported but do not undo the
// Postgres purge, so the API never serves purged evidence.
func (p *Purger) Run(ctx context.Context) (Stats, error) {
	var st Stats
	var errs []error
	for {
		n, att, err := p.purgeBatch(ctx, &errs)
		if err != nil {
			return st, err
		}
		st.Sessions += n
		st.Attachments += att
		if n < batchSize {
			break
		}
	}
	n, err := p.purgeArtifactCandidates(ctx, &errs)
	if err != nil {
		return st, err
	}
	st.ArtifactCandidates = n
	return st, errors.Join(errs...)
}

func (p *Purger) purgeBatch(ctx context.Context, errs *[]error) (int, int, error) {
	rows, err := p.Pool.Query(ctx, expiredSessions, batchSize)
	if err != nil {
		return 0, 0, err
	}
	sessions, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (expired, error) {
		var e expired
		return e, r.Scan(&e.id, &e.accountID, &e.projectID)
	})
	if err != nil || len(sessions) == 0 {
		return 0, 0, err
	}
	ids := make([]uuid.UUID, len(sessions))
	for i, s := range sessions {
		ids[i] = s.id
	}

	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	keys, err := deleteReturningKeys(ctx, tx,
		`DELETE FROM artifact_candidates WHERE session_id = ANY($1) RETURNING body_object_key`, ids)
	if err != nil {
		return 0, 0, err
	}
	attKeys, err := deleteReturningKeys(ctx, tx,
		`DELETE FROM attachments WHERE session_id = ANY($1) RETURNING COALESCE(CASE WHEN binary_stored THEN object_key END, '')`, ids)
	if err != nil {
		return 0, 0, err
	}
	for _, q := range sessionStatements {
		if _, err := tx.Exec(ctx, q, ids); err != nil {
			return 0, 0, fmt.Errorf("%s: %w", q, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}

	if p.Objects != nil {
		for _, s := range sessions {
			if err := p.Objects.DeletePrefix(ctx, objectstore.TranscriptPrefix(s.accountID, s.projectID, s.id)); err != nil {
				*errs = append(*errs, err)
			}
		}
		p.deleteKeys(ctx, append(keys, attKeys...), errs)
	}
	return len(sessions), len(attKeys), nil
}

func (p *Purger) purgeArtifactCandidates(ctx context.Context, errs *[]error) (int, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	keys, err := deleteReturningKeys(ctx, tx,
		`DELETE FROM artifact_candidates WHERE expires_at <= now() RETURNING body_object_key`)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	if p.Objects != nil {
		p.deleteKeys(ctx, keys, errs)
	}
	return len(keys), nil
}

func (p *Purger) deleteKeys(ctx context.Context, keys []string, errs *[]error) {
	for _, k := range keys {
		if k == "" {
			continue
		}
		if err := p.Objects.Delete(ctx, k); err != nil && !errors.Is(err, objectstore.ErrNotFound) {
			*errs = append(*errs, err)
		}
	}
}

func deleteReturningKeys(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", q, err)
	}
	return keys, nil
}
