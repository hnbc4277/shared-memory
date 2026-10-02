package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/index"
	"sharedmemory/internal/objectstore"
)

var ErrNotFound = errors.New("not found")

// Deleter cascades a project or an account across Postgres, MinIO,
// OpenSearch and Qdrant. Postgres goes first so the data is unreachable
// through the API even if a downstream store fails mid-way.
type Deleter struct {
	Pool    *pgxpool.Pool
	Objects objectstore.Store
	Index   *index.Service
}

// scoped tables in FK-safe order; each is filtered by account and,
// for project deletes, by project.
var projectStatements = []string{
	`DELETE FROM vault_candidates WHERE account_id = $1 AND (project_id = $2 OR session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2))`,
	`DELETE FROM vault_items WHERE account_id = $1 AND project_id = $2`,
	`DELETE FROM artifact_candidates WHERE account_id = $1 AND (project_id = $2 OR session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2))`,
	`DELETE FROM artifact_versions WHERE account_id = $1 AND artifact_id IN (SELECT id FROM artifacts WHERE account_id = $1 AND project_id = $2)`,
	`DELETE FROM artifacts WHERE account_id = $1 AND project_id = $2`,
	`UPDATE artifacts SET source_session_id = NULL WHERE account_id = $1 AND source_session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2)`,
	`UPDATE vault_items SET source_session_id = NULL WHERE account_id = $1 AND source_session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2)`,
	`DELETE FROM memories WHERE account_id = $1 AND (project_id = $2 OR source_session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2))`,
	`DELETE FROM attachments WHERE account_id = $1 AND (project_id = $2 OR session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2))`,
	`DELETE FROM events WHERE account_id = $1 AND session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2)`,
	`DELETE FROM messages WHERE account_id = $1 AND session_id IN (SELECT id FROM sessions WHERE account_id = $1 AND project_id = $2)`,
	`DELETE FROM sessions WHERE account_id = $1 AND project_id = $2`,
}

var accountStatements = []string{
	`DELETE FROM vault_candidates WHERE account_id = $1`,
	`DELETE FROM vault_items WHERE account_id = $1`,
	`DELETE FROM artifact_candidates WHERE account_id = $1`,
	`DELETE FROM artifact_versions WHERE account_id = $1`,
	`DELETE FROM artifacts WHERE account_id = $1`,
	`DELETE FROM memories WHERE account_id = $1`,
	`DELETE FROM profiles WHERE account_id = $1`,
	`DELETE FROM attachments WHERE account_id = $1`,
	`DELETE FROM events WHERE account_id = $1`,
	`DELETE FROM messages WHERE account_id = $1`,
	`DELETE FROM sessions WHERE account_id = $1`,
	`DELETE FROM projects WHERE account_id = $1`,
	`DELETE FROM account_policies WHERE account_id = $1`,
	`DELETE FROM api_keys WHERE account_id = $1`,
	`DELETE FROM audit_logs WHERE account_id = $1`,
	`DELETE FROM accounts WHERE id = $1`,
}

func exec(ctx context.Context, tx pgx.Tx, stmts []string, args ...any) error {
	for _, q := range stmts {
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	return nil
}

func (d *Deleter) DeleteProject(ctx context.Context, accountID, projectID uuid.UUID) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND account_id = $2)`,
		projectID, accountID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if err := exec(ctx, tx, projectStatements, accountID, projectID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND account_id = $2`, projectID, accountID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	var errs []error
	if d.Objects != nil {
		errs = append(errs, d.Objects.DeletePrefix(ctx, objectstore.ProjectPrefix(accountID, projectID)))
	}
	if d.Index != nil {
		errs = append(errs, d.Index.DeleteProject(ctx, accountID, projectID))
	}
	return errors.Join(errs...)
}

func (d *Deleter) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := exec(ctx, tx, accountStatements, accountID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	var errs []error
	if d.Objects != nil {
		errs = append(errs, d.Objects.DeletePrefix(ctx, objectstore.AccountPrefix(accountID)))
	}
	if d.Index != nil {
		errs = append(errs, d.Index.DeleteAccount(ctx, accountID))
	}
	return errors.Join(errs...)
}
