package index

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/google/uuid"

	"sharedmemory/internal/semantic"
)

// rrfK is the Reciprocal Rank Fusion constant used to merge keyword and vector rankings.
const rrfK = 60

// Service writes to and fuses results from both engines. Either engine may be
// down; callers get partial results plus a non-nil error describing the gap.
type Service struct {
	Text     TextIndex
	Vector   VectorIndex
	Embedder semantic.Embedder
}

func (s *Service) Ensure(ctx context.Context) error {
	var errs []error
	if s.Text != nil {
		errs = append(errs, s.Text.Ensure(ctx))
	}
	if s.Vector != nil {
		errs = append(errs, s.Vector.Ensure(ctx))
	}
	return errors.Join(errs...)
}

func (s *Service) IndexDoc(ctx context.Context, d Doc) error {
	var errs []error
	if s.Text != nil {
		errs = append(errs, s.Text.Index(ctx, d))
	}
	if s.Vector != nil {
		vec, err := s.Embedder.Embed(ctx, d.Body)
		if err != nil {
			errs = append(errs, err)
		} else {
			errs = append(errs, s.Vector.Upsert(ctx, d, vec))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) Search(ctx context.Context, q Query) ([]Hit, error) {
	var (
		wg              sync.WaitGroup
		textHits, vHits []Hit
		textErr, vecErr error
	)
	if s.Text != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			textHits, textErr = s.Text.Search(ctx, q)
		}()
	}
	if s.Vector != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vec, err := s.Embedder.Embed(ctx, q.Text)
			if err != nil {
				vecErr = err
				return
			}
			vq := q
			vq.Vector = vec
			vHits, vecErr = s.Vector.Search(ctx, vq)
		}()
	}
	wg.Wait()
	return Fuse(q.Limit, textHits, vHits), errors.Join(textErr, vecErr)
}

// Fuse merges ranked lists with Reciprocal Rank Fusion.
func Fuse(limit int, lists ...[]Hit) []Hit {
	type acc struct {
		hit   Hit
		score float64
	}
	byID := map[uuid.UUID]*acc{}
	for _, list := range lists {
		for rank, h := range list {
			a, ok := byID[h.ID]
			if !ok {
				a = &acc{hit: h}
				byID[h.ID] = a
			}
			a.score += 1.0 / float64(rrfK+rank+1)
		}
	}
	out := make([]Hit, 0, len(byID))
	for _, a := range byID {
		h := a.hit
		h.Score = a.score
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID.String() < out[j].ID.String()
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Service) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	var errs []error
	if s.Text != nil {
		errs = append(errs, s.Text.Delete(ctx, accountID, id))
	}
	if s.Vector != nil {
		errs = append(errs, s.Vector.Delete(ctx, accountID, id))
	}
	return errors.Join(errs...)
}

func (s *Service) DeleteProject(ctx context.Context, accountID, projectID uuid.UUID) error {
	var errs []error
	if s.Text != nil {
		errs = append(errs, s.Text.DeleteProject(ctx, accountID, projectID))
	}
	if s.Vector != nil {
		errs = append(errs, s.Vector.DeleteProject(ctx, accountID, projectID))
	}
	return errors.Join(errs...)
}

func (s *Service) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	var errs []error
	if s.Text != nil {
		errs = append(errs, s.Text.DeleteAccount(ctx, accountID))
	}
	if s.Vector != nil {
		errs = append(errs, s.Vector.DeleteAccount(ctx, accountID))
	}
	return errors.Join(errs...)
}
