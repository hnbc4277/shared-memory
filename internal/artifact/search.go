package artifact

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"sharedmemory/internal/index"
	"sharedmemory/internal/semantic"
	"sharedmemory/internal/vault"
)

const indexedRunes = 4000

// SearchDoc is the L3 projection of an artifact. The indexed text is always
// redacted, even if the user chose to keep a secret in the body.
func SearchDoc(accountID uuid.UUID, d Detail) index.Doc {
	a := d.Artifact
	scope := semantic.ScopeGlobal
	if a.ProjectID != nil {
		scope = semantic.ScopeProject
	}
	return index.Doc{ID: a.ID, DocType: index.DocArtifact, AccountID: accountID, ProjectID: a.ProjectID,
		Scope: scope, Kind: a.Type, Body: vault.Redact(a.Title + "\n" + a.Summary + "\n" + semantic.Truncate(d.Body, indexedRunes)),
		SourceSessionID: a.SourceSessionID, SourceAgentPath: a.SourceAgentPath, CreatedAt: a.CreatedAt}
}

type Ref struct {
	AccountID uuid.UUID
	ID        uuid.UUID
}

// Searchable lists artifacts whose account allows artifact search (accounts
// without a policy row use the default, which allows it).
func (s *Store) Searchable(ctx context.Context) ([]Ref, error) {
	rows, err := s.pool.Query(ctx, `SELECT a.account_id, a.id FROM artifacts a
		LEFT JOIN account_policies p ON p.account_id = a.account_id
		WHERE COALESCE(p.artifact_searchable, TRUE) ORDER BY a.account_id, a.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Ref, error) {
		var ref Ref
		return ref, r.Scan(&ref.AccountID, &ref.ID)
	})
}
