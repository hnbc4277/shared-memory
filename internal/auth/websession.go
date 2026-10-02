package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const WebSessionTTL = 7 * 24 * time.Hour

type WebSession struct {
	ID        uuid.UUID `json:"id"`
	AccountID uuid.UUID `json:"account_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token,omitempty"` // plaintext only at creation
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "wss_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// LoginWithPassword verifies credentials and issues a web console session token.
func (s *Store) LoginWithPassword(ctx context.Context, email, password string) (WebSession, error) {
	accountID, err := s.Login(ctx, email, password)
	if err != nil {
		return WebSession{}, err
	}
	return s.CreateWebSession(ctx, accountID)
}

func (s *Store) CreateWebSession(ctx context.Context, accountID uuid.UUID) (WebSession, error) {
	token, err := newSessionToken()
	if err != nil {
		return WebSession{}, err
	}
	id := uuid.New()
	expires := time.Now().UTC().Add(WebSessionTTL)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO web_sessions (id, account_id, token_hash, expires_at, last_seen_at)
		VALUES ($1,$2,$3,$4,now())`, id, accountID, hashSessionToken(token), expires)
	if err != nil {
		return WebSession{}, fmt.Errorf("insert web session: %w", err)
	}
	return WebSession{ID: id, AccountID: accountID, ExpiresAt: expires, Token: token}, nil
}

func (s *Store) AuthenticateWebSession(ctx context.Context, token string) (uuid.UUID, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return uuid.Nil, ErrUnauthorized
	}
	var accountID uuid.UUID
	var expires time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT account_id, expires_at FROM web_sessions WHERE token_hash = $1`, hashSessionToken(token),
	).Scan(&accountID, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrUnauthorized
	}
	if err != nil {
		return uuid.Nil, err
	}
	if time.Now().UTC().After(expires) {
		_, _ = s.pool.Exec(ctx, `DELETE FROM web_sessions WHERE token_hash = $1`, hashSessionToken(token))
		return uuid.Nil, ErrUnauthorized
	}
	_, _ = s.pool.Exec(ctx, `UPDATE web_sessions SET last_seen_at = now() WHERE token_hash = $1`, hashSessionToken(token))
	return accountID, nil
}

func (s *Store) RevokeWebSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM web_sessions WHERE token_hash = $1`, hashSessionToken(token))
	return err
}
