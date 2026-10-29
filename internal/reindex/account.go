package reindex

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/index"
	"sharedmemory/internal/kb"
	"sharedmemory/internal/semantic"
)

// EmbeddingSettings is the subset of the account embedding store used after a rebuild.
type EmbeddingSettings interface {
	Get(ctx context.Context, accountID uuid.UUID) (semantic.Settings, error)
	Upsert(ctx context.Context, accountID uuid.UUID, st semantic.Settings, apiKeyPlain ...string) error
}

// RunAccount rebuilds one account's vector collection from Postgres and then
// clears reindex_required. The flag stays set if any document fails.
func (r *Reindexer) RunAccount(ctx context.Context, accountID uuid.UUID) (int, error) {
	if r.Resolver == nil {
		return 0, fmt.Errorf("embedding resolver is not configured")
	}
	res, err := r.Resolver.ForAccount(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if qdr, ok := r.Index.Vector.(*index.Qdrant); ok {
		coll := res.Collection
		if coll == "" {
			coll = semantic.CollectionName(accountID, res.Model, res.Dim)
		}
		q := qdr.ForCollection(coll, res.Dim)
		if err := q.Ensure(ctx); err != nil {
			return 0, err
		}
		if err := q.DeleteAccount(ctx, accountID); err != nil {
			return 0, err
		}
	}
	ctx = index.WithForceVector(ctx)
	n, err := r.reindexAccountDocs(ctx, accountID)
	if err != nil {
		return n, err
	}
	if err := r.markReindexed(ctx, accountID, res); err != nil {
		return n, err
	}
	return n, nil
}

func (r *Reindexer) reindexAccountDocs(ctx context.Context, accountID uuid.UUID) (int, error) {
	n, err := r.reindexMemories(ctx, accountID)
	if err != nil {
		return n, err
	}
	an, err := r.reindexArtifacts(ctx, accountID)
	n += an
	if err != nil {
		return n, err
	}
	kn, err := r.reindexKB(ctx, accountID)
	return n + kn, err
}

func (r *Reindexer) reindexMemories(ctx context.Context, accountID uuid.UUID) (int, error) {
	if r.Pool == nil || r.Index == nil {
		return 0, nil
	}
	rows, err := r.Pool.Query(ctx, `SELECT id, project_id, scope, kind, body, source_session_id, source_connection_id, COALESCE(source_agent_path,''), created_at
		FROM memories WHERE account_id = $1 AND NOT invalidated ORDER BY id`, accountID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var d index.Doc
		d.DocType = index.DocMemory
		d.AccountID = accountID
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Scope, &d.Kind, &d.Body, &d.SourceSessionID, &d.SourceConnectionID, &d.SourceAgentPath, &d.CreatedAt); err != nil {
			return n, err
		}
		if err := r.Index.IndexDoc(ctx, d); err != nil {
			return n, fmt.Errorf("reindex memory %s: %w", d.ID, err)
		}
		n++
	}
	return n, rows.Err()
}

func (r *Reindexer) reindexArtifacts(ctx context.Context, accountID uuid.UUID) (int, error) {
	if r.Artifacts == nil || r.Index == nil {
		return 0, nil
	}
	refs, err := r.Artifacts.Searchable(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ref := range refs {
		if ref.AccountID != accountID {
			continue
		}
		d, err := r.Artifacts.Get(ctx, accountID, ref.ID)
		if err != nil {
			return n, fmt.Errorf("reindex artifact %s: %w", ref.ID, err)
		}
		if err := r.Index.IndexDoc(ctx, artifact.SearchDoc(accountID, d)); err != nil {
			return n, fmt.Errorf("reindex artifact %s: %w", ref.ID, err)
		}
		n++
	}
	return n, nil
}

func (r *Reindexer) reindexKB(ctx context.Context, accountID uuid.UUID) (int, error) {
	if r.Pool == nil || r.Index == nil {
		return 0, nil
	}
	rows, err := r.Pool.Query(ctx, `SELECT c.id, c.body, COALESCE(d.title, '')
		FROM kb_chunks c
		JOIN kb_documents d ON d.id = c.document_id AND d.account_id = c.account_id
		WHERE c.account_id = $1 AND d.status = $2
		ORDER BY c.id`, accountID, kb.StatusReady)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	now := time.Now().UTC()
	for rows.Next() {
		var id uuid.UUID
		var body, title string
		if err := rows.Scan(&id, &body, &title); err != nil {
			return n, err
		}
		if err := r.Index.IndexDoc(ctx, index.Doc{
			ID: id, DocType: index.DocKBChunk, AccountID: accountID, Kind: title, Body: body, CreatedAt: now,
		}); err != nil {
			return n, fmt.Errorf("reindex kb chunk %s: %w", id, err)
		}
		n++
	}
	return n, rows.Err()
}

func (r *Reindexer) markReindexed(ctx context.Context, accountID uuid.UUID, res semantic.Resolved) error {
	if r.Settings == nil {
		return nil
	}
	st, err := r.Settings.Get(ctx, accountID)
	if err != nil {
		return err
	}
	if !embeddingRow(st) {
		return nil
	}
	st.ActiveFingerprint = semantic.Fingerprint(res.Protocol, res.Model, res.Dim, res.BaseURL)
	st.ReindexRequired = false
	return r.Settings.Upsert(ctx, accountID, st)
}

func embeddingRow(st semantic.Settings) bool {
	return st.Enabled || st.ReindexRequired || st.ActiveFingerprint != "" || st.BaseURL != "" || st.APIKeySet || st.Dim > 0 || st.Protocol != ""
}
