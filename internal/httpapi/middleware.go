package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"sharedmemory/internal/auth"
	"sharedmemory/internal/ratelimit"
)

type ctxKey int

const (
	accountIDKey ctxKey = 1
	deviceIDKey  ctxKey = 2
	apiKeyIDKey  ctxKey = 3
)

func AccountIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(accountIDKey).(uuid.UUID)
	return id, ok
}

func DeviceIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(deviceIDKey).(uuid.UUID)
	return id, ok
}

func APIKeyIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(apiKeyIDKey).(uuid.UUID)
	return id, ok
}

func withAccountID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, accountIDKey, id)
}

func withDeviceID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, deviceIDKey, id)
}

func withAPIKeyID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, apiKeyIDKey, id)
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
			ctx := r.Context()
			if strings.HasPrefix(token, "wss_") {
				accountID, err := store.AuthenticateWebSession(ctx, token)
				if err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r.WithContext(withAccountID(ctx, accountID)))
				return
			}
			accountID, apiKeyID, deviceID, err := store.AuthenticateAPIKeyFull(ctx, token)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			authCtx := withAccountID(ctx, accountID)
			if apiKeyID != nil {
				authCtx = withAPIKeyID(authCtx, *apiKeyID)
			}
			if deviceID != nil {
				authCtx = withDeviceID(authCtx, *deviceID)
			}
			next.ServeHTTP(w, r.WithContext(authCtx))
		})
	}
}

// AuthEmailRateLimit applies per-email login limits; returns false after writing 429.
func AuthEmailRateLimit(limiter *ratelimit.Limiter, w http.ResponseWriter, ctx context.Context, email string) bool {
	if limiter == nil {
		return true
	}
	ok, err := limiter.AllowEmail(ctx, strings.ToLower(strings.TrimSpace(email)), "auth_login_email")
	if err != nil || ok {
		return true
	}
	writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
	return false
}

// RateLimit rejects when the account exceeds Limit for the named route.
func RateLimit(limiter *ratelimit.Limiter, route string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accountID, ok := AccountIDFromContext(r.Context())
			if !ok || limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			okAllow, err := limiter.Allow(r.Context(), accountID, route)
			if err != nil || okAllow {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		})
	}
}
