package semantic

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/vault"
)

type Store struct {
	pool   *pgxpool.Pool
	cipher *vault.Cipher
}

func NewStore(pool *pgxpool.Pool, cipher *vault.Cipher) *Store {
	return &Store{pool: pool, cipher: cipher}
}

func (s *Store) Get(ctx context.Context, accountID uuid.UUID) (Settings, error) {
	var st Settings
	var ct, nonce []byte
	err := s.pool.QueryRow(ctx, `
		SELECT enabled, protocol, base_url, model, dim,
			api_key_ciphertext, api_key_nonce,
			COALESCE(active_fingerprint, ''), reindex_required
		FROM account_embedding_settings WHERE account_id = $1`, accountID,
	).Scan(&st.Enabled, &st.Protocol, &st.BaseURL, &st.Model, &st.Dim,
		&ct, &nonce, &st.ActiveFingerprint, &st.ReindexRequired)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("get embedding settings: %w", err)
	}
	st.APIKeySet = len(ct) > 0 && len(nonce) > 0
	return st, nil
}

// Upsert replaces the row for accountID. When apiKeyPlain is omitted, any
// existing encrypted key is kept. A single empty string clears the stored key.
func (s *Store) Upsert(ctx context.Context, accountID uuid.UUID, st Settings, apiKeyPlain ...string) error {
	var ct, nonce []byte
	switch len(apiKeyPlain) {
	case 0:
		err := s.pool.QueryRow(ctx, `
			SELECT api_key_ciphertext, api_key_nonce
			FROM account_embedding_settings WHERE account_id = $1`, accountID,
		).Scan(&ct, &nonce)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read embedding api key: %w", err)
		}
	case 1:
		if apiKeyPlain[0] != "" {
			var err error
			ct, nonce, err = s.cipher.Encrypt(accountID, apiKeyPlain[0])
			if err != nil {
				return fmt.Errorf("encrypt embedding api key: %w", err)
			}
		}
	default:
		return fmt.Errorf("at most one api key argument")
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO account_embedding_settings (
			account_id, enabled, protocol, base_url, model, dim,
			api_key_ciphertext, api_key_nonce,
			active_fingerprint, reindex_required, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (account_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			protocol = EXCLUDED.protocol,
			base_url = EXCLUDED.base_url,
			model = EXCLUDED.model,
			dim = EXCLUDED.dim,
			api_key_ciphertext = EXCLUDED.api_key_ciphertext,
			api_key_nonce = EXCLUDED.api_key_nonce,
			active_fingerprint = EXCLUDED.active_fingerprint,
			reindex_required = EXCLUDED.reindex_required,
			updated_at = now()`,
		accountID, st.Enabled, st.Protocol, st.BaseURL, st.Model, st.Dim,
		ct, nonce, nullIfEmpty(st.ActiveFingerprint), st.ReindexRequired,
	)
	if err != nil {
		return fmt.Errorf("upsert embedding settings: %w", err)
	}
	return nil
}

func (s *Store) Clear(ctx context.Context, accountID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM account_embedding_settings WHERE account_id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("clear embedding settings: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
