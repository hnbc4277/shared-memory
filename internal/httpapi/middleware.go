package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"sharedmemory/internal/auth"
)

type ctxKey int

const (
	accountIDKey ctxKey = 1
	deviceIDKey  ctxKey = 2
)

func AccountIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(accountIDKey).(uuid.UUID)
	return id, ok
}

func DeviceIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(deviceIDKey).(uuid.UUID)
	return id, ok
}

func withAccountID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, accountIDKey, id)
}

func withDeviceID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, deviceIDKey, id)
}

func BearerAuth(store *auth.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			accountID, deviceID, err := store.AuthenticateAPIKeyFull(r.Context(), token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := withAccountID(r.Context(), accountID)
			if deviceID != nil {
				ctx = withDeviceID(ctx, *deviceID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
