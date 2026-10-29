package index

import "context"

type forceVectorKey struct{}

// WithForceVector marks ctx so IndexDoc writes vectors even when the account
// has reindex_required (used by account embedding rebuild).
func WithForceVector(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceVectorKey{}, true)
}

func forceVector(ctx context.Context) bool {
	v, _ := ctx.Value(forceVectorKey{}).(bool)
	return v
}
