package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"sharedmemory/internal/auth"
)

const webSessionCookie = "sm_session"

func setWebSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expires.UTC(),
	})
}

func clearWebSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
	})
}

func bearerOrCookieToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if c, err := r.Cookie(webSessionCookie); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

func authenticateRequest(ctx context.Context, store *auth.Store, token string) (context.Context, bool) {
	if token == "" {
		return ctx, false
	}
	if strings.HasPrefix(token, "wss_") {
		accountID, err := store.AuthenticateWebSession(ctx, token)
		if err != nil {
			return ctx, false
		}
		return withAccountID(ctx, accountID), true
	}
	accountID, apiKeyID, deviceID, err := store.AuthenticateAPIKeyFull(ctx, token)
	if err != nil {
		return ctx, false
	}
	authCtx := withAccountID(ctx, accountID)
	if apiKeyID != nil {
		authCtx = withAPIKeyID(authCtx, *apiKeyID)
	}
	if deviceID != nil {
		authCtx = withDeviceID(authCtx, *deviceID)
	}
	return authCtx, true
}
