package policy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Policy struct {
	StoreTranscript     bool `json:"store_transcript"`
	StoreAttachments    bool `json:"store_attachments"`
	InjectEnabled       bool `json:"inject_enabled"`
	VaultEnabled        bool `json:"vault_enabled"`
	ArtifactSearchable  bool `json:"artifact_searchable"`
	CloudVisionEnabled  bool `json:"cloud_vision_enabled"`
	RetentionDays       int  `json:"retention_days"`
}

func Default() Policy {
	return Policy{
		StoreTranscript:    true,
		StoreAttachments:   true,
		InjectEnabled:      true,
		VaultEnabled:       true,
		ArtifactSearchable: true,
		CloudVisionEnabled: true,
	}
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Get(ctx context.Context, accountID uuid.UUID) (Policy, error) {
	var p Policy
	err := s.pool.QueryRow(ctx, `
		INSERT INTO account_policies (account_id) VALUES ($1)
		ON CONFLICT (account_id) DO UPDATE SET account_id = EXCLUDED.account_id
		RETURNING store_transcript, store_attachments, inject_enabled,
		          vault_enabled, artifact_searchable, cloud_vision_enabled, retention_days`,
		accountID,
	).Scan(&p.StoreTranscript, &p.StoreAttachments, &p.InjectEnabled,
		&p.VaultEnabled, &p.ArtifactSearchable, &p.CloudVisionEnabled, &p.RetentionDays)
	if err != nil {
		return Policy{}, fmt.Errorf("get policy: %w", err)
	}
	return p, nil
}

func (s *Store) Update(ctx context.Context, accountID uuid.UUID, p Policy) error {
	if p.RetentionDays < 0 {
		return fmt.Errorf("retention_days must be >= 0")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO account_policies (account_id, store_transcript, store_attachments,
			inject_enabled, vault_enabled, artifact_searchable, cloud_vision_enabled, retention_days)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (account_id) DO UPDATE SET
			store_transcript = EXCLUDED.store_transcript,
			store_attachments = EXCLUDED.store_attachments,
			inject_enabled = EXCLUDED.inject_enabled,
			vault_enabled = EXCLUDED.vault_enabled,
			artifact_searchable = EXCLUDED.artifact_searchable,
			cloud_vision_enabled = EXCLUDED.cloud_vision_enabled,
			retention_days = EXCLUDED.retention_days`,
		accountID, p.StoreTranscript, p.StoreAttachments, p.InjectEnabled,
		p.VaultEnabled, p.ArtifactSearchable, p.CloudVisionEnabled, p.RetentionDays,
	)
	if err != nil {
		return fmt.Errorf("update policy: %w", err)
	}
	return nil
}
