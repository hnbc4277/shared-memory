// Package device implements the OAuth-style device-code login used by smctl
// and the local daemon so agents never type account passwords.
package device

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/auth"
)

var (
	ErrPending  = errors.New("authorization_pending")
	ErrExpired  = errors.New("expired_token")
	ErrNotFound = errors.New("not found")
	ErrRedeemed = errors.New("already redeemed")
	ErrInvalid  = errors.New("invalid user_code")
)

const (
	codeTTL      = 10 * time.Minute
	pollInterval = 5
	userAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

type Store struct {
	pool *pgxpool.Pool
	auth *auth.Store
}

func NewStore(pool *pgxpool.Pool, authStore *auth.Store) *Store {
	return &Store{pool: pool, auth: authStore}
}

type Code struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type Device struct {
	ID            uuid.UUID  `json:"id"`
	ClientName    string     `json:"client_name"`
	Agent         string     `json:"agent"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
	DaemonVersion string     `json:"daemon_version,omitempty"`
	SpoolPending  int        `json:"spool_pending"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type Token struct {
	APIKey    string    `json:"api_key"`
	DeviceID  uuid.UUID `json:"device_id"`
	AccountID uuid.UUID `json:"account_id"`
}

func (s *Store) CreateCode(ctx context.Context, clientName, agent, verifyBase string) (Code, error) {
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		clientName = "device"
	}
	agent = strings.TrimSpace(agent)
	deviceCode, err := randomToken(32)
	if err != nil {
		return Code{}, err
	}
	userCode, err := randomUserCode(8)
	if err != nil {
		return Code{}, err
	}
	expires := time.Now().UTC().Add(codeTTL)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO device_codes (device_code, user_code, client_name, agent, expires_at, interval_seconds)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		deviceCode, userCode, clientName, agent, expires, pollInterval)
	if err != nil {
		return Code{}, err
	}
	base := strings.TrimRight(verifyBase, "/")
	if base == "" {
		base = "http://127.0.0.1:5173"
	}
	uri := base + "/device"
	return Code{
		DeviceCode: deviceCode, UserCode: userCode,
		VerificationURI: uri, VerificationURIComplete: uri + "?user_code=" + userCode,
		ExpiresIn: int(codeTTL.Seconds()), Interval: pollInterval,
	}, nil
}

func (s *Store) Approve(ctx context.Context, accountID uuid.UUID, userCode string) error {
	userCode = NormalizeUserCode(userCode)
	if userCode == "" {
		return ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var deviceCode, clientName, agent, status string
	var expires time.Time
	err = tx.QueryRow(ctx, `
		SELECT device_code, client_name, agent, status, expires_at
		FROM device_codes WHERE user_code = $1 FOR UPDATE`, userCode,
	).Scan(&deviceCode, &clientName, &agent, &status, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending" {
		return ErrRedeemed
	}
	if time.Now().UTC().After(expires) {
		_, _ = tx.Exec(ctx, `UPDATE device_codes SET status = 'expired' WHERE device_code = $1`, deviceCode)
		_ = tx.Commit(ctx)
		return ErrExpired
	}

	deviceID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO devices (id, account_id, client_name, agent, last_seen_at)
		VALUES ($1,$2,$3,$4,now())`, deviceID, accountID, clientName, agent); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	keyName := "device:" + clientName
	plaintext, err := s.auth.CreateAPIKeyForDevice(ctx, accountID, keyName, &deviceID)
	if err != nil {
		_, _ = s.pool.Exec(ctx, `UPDATE devices SET revoked_at = now() WHERE id = $1`, deviceID)
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE device_codes
		SET status = 'approved', account_id = $2, device_id = $3, issued_key = $4
		WHERE device_code = $1 AND status = 'pending'`,
		deviceCode, accountID, deviceID, plaintext)
	return err
}

func (s *Store) Poll(ctx context.Context, deviceCode string) (Token, error) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return Token{}, ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Token{}, err
	}
	defer tx.Rollback(ctx)

	var status, issued string
	var accountID, deviceID *uuid.UUID
	var expires time.Time
	err = tx.QueryRow(ctx, `
		SELECT status, COALESCE(issued_key, ''), account_id, device_id, expires_at
		FROM device_codes WHERE device_code = $1 FOR UPDATE`, deviceCode,
	).Scan(&status, &issued, &accountID, &deviceID, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, err
	}
	now := time.Now().UTC()
	if status == "pending" {
		if now.After(expires) {
			_, _ = tx.Exec(ctx, `UPDATE device_codes SET status = 'expired' WHERE device_code = $1`, deviceCode)
			_ = tx.Commit(ctx)
			return Token{}, ErrExpired
		}
		return Token{}, ErrPending
	}
	if status == "expired" {
		return Token{}, ErrExpired
	}
	if status == "redeemed" {
		return Token{}, ErrRedeemed
	}
	if status != "approved" || issued == "" || accountID == nil || deviceID == nil {
		return Token{}, ErrPending
	}
	_, err = tx.Exec(ctx, `
		UPDATE device_codes SET status = 'redeemed', issued_key = NULL WHERE device_code = $1`, deviceCode)
	if err != nil {
		return Token{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Token{}, err
	}
	return Token{APIKey: issued, DeviceID: *deviceID, AccountID: *accountID}, nil
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID) ([]Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, client_name, agent, last_seen_at, COALESCE(daemon_version, ''), spool_pending, created_at, revoked_at
		FROM devices WHERE account_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.ClientName, &d.Agent, &d.LastSeenAt, &d.DaemonVersion, &d.SpoolPending, &d.CreatedAt, &d.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Revoke(ctx context.Context, accountID, deviceID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET revoked_at = now() WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`,
		deviceID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM api_keys WHERE device_id = $1 AND account_id = $2`, deviceID, accountID)
	return err
}

func (s *Store) HeartbeatDevice(ctx context.Context, accountID, deviceID uuid.UUID, daemonVersion string, spoolPending int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE devices SET last_seen_at = now(), daemon_version = $3, spool_pending = $4
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`,
		deviceID, accountID, daemonVersion, spoolPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func NormalizeUserCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomUserCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = userAlphabet[int(b[i])%len(userAlphabet)]
	}
	return string(out), nil
}
