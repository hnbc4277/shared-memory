package reindex

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/artifact"
	"sharedmemory/internal/index"
	"sharedmemory/internal/memory"
)

// Reindexer backfills a vector collection from Postgres, the source of truth.
// Each collection is tied to one embedder, so switching embedders means a new
// collection that starts empty and is filled exactly once.
type Reindexer struct {
	Pool       *pgxpool.Pool
	Index      *index.Service
	Memories   *memory.Store
	Artifacts  *artifact.Store
	Collection string
}

func (r *Reindexer) Needed(ctx context.Context) (bool, error) {
	var done bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM index_state WHERE collection = $1)`, r.Collection).Scan(&done)
	return !done, err
}

func (r *Reindexer) Run(ctx context.Context) (int, error) {
	n, err := r.Memories.ReindexAll(ctx)
	if err != nil {
		return n, err
	}
	refs, err := r.Artifacts.Searchable(ctx)
	if err != nil {
		return n, err
	}
	for _, ref := range refs {
		d, err := r.Artifacts.Get(ctx, ref.AccountID, ref.ID)
		if err != nil {
			return n, fmt.Errorf("reindex artifact %s: %w", ref.ID, err)
		}
		if err := r.Index.IndexDoc(ctx, artifact.SearchDoc(ref.AccountID, d)); err != nil {
			return n, fmt.Errorf("reindex artifact %s: %w", ref.ID, err)
		}
		n++
	}
	_, err = r.Pool.Exec(ctx, `INSERT INTO index_state (collection, docs) VALUES ($1, $2)
		ON CONFLICT (collection) DO UPDATE SET reindexed_at = now(), docs = EXCLUDED.docs`, r.Collection, n)
	return n, err
}
