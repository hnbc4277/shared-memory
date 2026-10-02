package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnauthorized = errors.New("unauthorized")

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateAccount(ctx context.Context, email, password string) (uuid.UUID, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return uuid.Nil, fmt.Errorf("email and password are required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO accounts (id, email, password_hash) VALUES ($1, $2, $3)`,
		id, email, hash,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert account: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO account_policies (account_id) VALUES ($1)`,
		id,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert default policy: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, accountID uuid.UUID, name string) (string, error) {
	plaintext, err := NewAPIKeyPlaintext()
	if err != nil {
		return "", err
	}
	keyHash := HashAPIKey(plaintext)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, account_id, key_hash, name) VALUES ($1, $2, $3, $4)`,
		uuid.New(), accountID, keyHash, name,
	)
	if err != nil {
		return "", fmt.Errorf("insert api key: %w", err)
	}
	return plaintext, nil
}

func (s *Store) AuthenticateAPIKey(ctx context.Context, plaintextKey string) (uuid.UUID, error) {
	plaintextKey = strings.TrimSpace(plaintextKey)
	if plaintextKey == "" {
		return uuid.Nil, ErrUnauthorized
	}
	keyHash := HashAPIKey(plaintextKey)
	var accountID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT account_id FROM api_keys WHERE key_hash = $1`,
		keyHash,
	).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrUnauthorized
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lookup api key: %w", err)
	}
	return accountID, nil
}

// VerifyPassword re-authenticates a user, e.g. to unlock vault secrets.
func (s *Store) VerifyPassword(ctx context.Context, accountID uuid.UUID, password string) error {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM accounts WHERE id = $1`, accountID).Scan(&hash)
	if err != nil {
		return ErrUnauthorized
	}
	if CheckPassword(hash, password) != nil {
		return ErrUnauthorized
	}
	return nil
}

func (s *Store) Login(ctx context.Context, email, password string) (uuid.UUID, error) {
	var id uuid.UUID
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, password_hash FROM accounts WHERE email = $1`,
		strings.TrimSpace(strings.ToLower(email)),
	).Scan(&id, &hash)
	if err != nil || CheckPassword(hash, password) != nil {
		return uuid.Nil, ErrUnauthorized
	}
	return id, nil
}

type APIKeyInfo struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListAPIKeys(ctx context.Context, accountID uuid.UUID) ([]APIKeyInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, COALESCE(name, ''), created_at FROM api_keys WHERE account_id = $1 ORDER BY created_at`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKeyInfo{}
	for rows.Next() {
		var k APIKeyInfo
		if err := rows.Scan(&k.ID, &k.Name, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, accountID, keyID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND account_id = $2`, keyID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

// Register creates an account and returns one plaintext API key (once).
func (s *Store) Register(ctx context.Context, email, password string) (accountID uuid.UUID, apiKey string, err error) {
	accountID, err = s.CreateAccount(ctx, email, password)
	if err != nil {
		return uuid.Nil, "", err
	}
	apiKey, err = s.CreateAPIKey(ctx, accountID, "default")
	if err != nil {
		return uuid.Nil, "", err
	}
	return accountID, apiKey, nil
}
