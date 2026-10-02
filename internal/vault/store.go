package vault

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

var ErrNotFound = errors.New("vault entry not found")

const (
	ConsentPrompt = "prompt"
	ConsentManual = "manual"
)

// Candidate is a pending credential awaiting user consent. The secret stays
// encrypted and is never returned by list endpoints.
type Candidate struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID *uuid.UUID `json:"project_id"`
	SessionID *uuid.UUID `json:"session_id"`
	AgentPath string     `json:"agent_path"`
	Title     string     `json:"title"`
	SiteURL   string     `json:"site_url"`
	Username  string     `json:"username"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

type Item struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       *uuid.UUID `json:"project_id"`
	Title           string     `json:"title"`
	SiteURL         string     `json:"site_url"`
	Username        string     `json:"username"`
	SourceSessionID *uuid.UUID `json:"source_session_id"`
	SourceAgentPath string     `json:"source_agent_path"`
	ConsentMethod   string     `json:"consent_method"`
	ConsentedAt     time.Time  `json:"consented_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ItemInput struct {
	ProjectID *uuid.UUID `json:"project_id"`
	Title     string     `json:"title"`
	SiteURL   string     `json:"site_url"`
	Username  string     `json:"username"`
	Secret    string     `json:"secret"`
}

type Source struct {
	ProjectID *uuid.UUID
	SessionID uuid.UUID
	MessageID uuid.UUID
	AgentPath string
}

type Store struct {
	pool   *pgxpool.Pool
	cipher *Cipher
}

func NewStore(pool *pgxpool.Pool, cipher *Cipher) *Store {
	return &Store{pool: pool, cipher: cipher}
}

func (s *Store) CreateCandidates(ctx context.Context, accountID uuid.UUID, src Source, found []Detected) ([]Candidate, error) {
	var out []Candidate
	for _, d := range found {
		ct, nonce, err := s.cipher.Encrypt(accountID, d.Secret)
		if err != nil {
			return out, err
		}
		c := Candidate{ID: uuid.New(), ProjectID: src.ProjectID, SessionID: &src.SessionID,
			AgentPath: src.AgentPath, Title: d.Title, SiteURL: d.SiteURL, Username: d.Username}
		err = s.pool.QueryRow(ctx, `
			INSERT INTO vault_candidates (id, account_id, project_id, session_id, message_id, agent_path,
				title, site_url, username, secret_cipher, secret_nonce)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING created_at, expires_at`,
			c.ID, accountID, src.ProjectID, src.SessionID, src.MessageID, src.AgentPath,
			c.Title, c.SiteURL, c.Username, ct, nonce,
		).Scan(&c.CreatedAt, &c.ExpiresAt)
		if err != nil {
			return out, fmt.Errorf("insert vault candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *Store) ListCandidates(ctx context.Context, accountID uuid.UUID) ([]Candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, session_id, agent_path, title, site_url, username, created_at, expires_at
		FROM vault_candidates WHERE account_id = $1 AND expires_at > now() ORDER BY created_at DESC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.SessionID, &c.AgentPath, &c.Title, &c.SiteURL,
			&c.Username, &c.CreatedAt, &c.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type AcceptOverrides struct {
	Title    string `json:"title"`
	SiteURL  string `json:"site_url"`
	Username string `json:"username"`
}

// Accept moves a candidate into the vault after explicit user consent.
func (s *Store) Accept(ctx context.Context, accountID, candidateID uuid.UUID, o AcceptOverrides) (Item, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback(ctx)
	var (
		projectID, sessionID   *uuid.UUID
		agentPath, title, site string
		user                   string
		ct, nonce              []byte
	)
	err = tx.QueryRow(ctx, `
		DELETE FROM vault_candidates WHERE id = $1 AND account_id = $2 AND expires_at > now()
		RETURNING project_id, session_id, agent_path, title, site_url, username, secret_cipher, secret_nonce`,
		candidateID, accountID,
	).Scan(&projectID, &sessionID, &agentPath, &title, &site, &user, &ct, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	title, site, user = pick(o.Title, title), pick(o.SiteURL, site), pick(o.Username, user)
	it := Item{ID: uuid.New(), ProjectID: projectID, Title: title, SiteURL: site, Username: user,
		SourceSessionID: sessionID, SourceAgentPath: agentPath, ConsentMethod: ConsentPrompt}
	err = tx.QueryRow(ctx, `
		INSERT INTO vault_items (id, account_id, project_id, title, site_url, username, secret_cipher,
			secret_nonce, source_session_id, source_agent_path, consent_method)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING consented_at, updated_at`,
		it.ID, accountID, projectID, title, site, user, ct, nonce, sessionID, agentPath, ConsentPrompt,
	).Scan(&it.ConsentedAt, &it.UpdatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("insert vault item: %w", err)
	}
	return it, tx.Commit(ctx)
}

func (s *Store) Reject(ctx context.Context, accountID, candidateID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vault_candidates WHERE id = $1 AND account_id = $2`, candidateID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) PurgeExpired(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM vault_candidates WHERE expires_at <= now()`)
	return err
}

func (s *Store) Create(ctx context.Context, accountID uuid.UUID, in ItemInput) (Item, error) {
	if strings.TrimSpace(in.Secret) == "" {
		return Item{}, fmt.Errorf("secret is required")
	}
	ct, nonce, err := s.cipher.Encrypt(accountID, in.Secret)
	if err != nil {
		return Item{}, err
	}
	it := Item{ID: uuid.New(), ProjectID: in.ProjectID, Title: strings.TrimSpace(in.Title),
		SiteURL: strings.TrimSpace(in.SiteURL), Username: strings.TrimSpace(in.Username), ConsentMethod: ConsentManual}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO vault_items (id, account_id, project_id, title, site_url, username, secret_cipher,
			secret_nonce, consent_method)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING consented_at, updated_at`,
		it.ID, accountID, in.ProjectID, it.Title, it.SiteURL, it.Username, ct, nonce, ConsentManual,
	).Scan(&it.ConsentedAt, &it.UpdatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("insert vault item: %w", err)
	}
	return it, nil
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, title, site_url, username, source_session_id, source_agent_path,
		       consent_method, consented_at, updated_at
		FROM vault_items WHERE account_id = $1 ORDER BY updated_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.ProjectID, &it.Title, &it.SiteURL, &it.Username, &it.SourceSessionID,
			&it.SourceAgentPath, &it.ConsentMethod, &it.ConsentedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) Reveal(ctx context.Context, accountID, itemID uuid.UUID) (string, error) {
	var ct, nonce []byte
	err := s.pool.QueryRow(ctx,
		`SELECT secret_cipher, secret_nonce FROM vault_items WHERE id = $1 AND account_id = $2`,
		itemID, accountID,
	).Scan(&ct, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return s.cipher.Decrypt(accountID, ct, nonce)
}

func (s *Store) Update(ctx context.Context, accountID, itemID uuid.UUID, in ItemInput) error {
	var ct, nonce []byte
	if in.Secret != "" {
		var err error
		if ct, nonce, err = s.cipher.Encrypt(accountID, in.Secret); err != nil {
			return err
		}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vault_items SET
			title = $3, site_url = $4, username = $5,
			secret_cipher = COALESCE($6, secret_cipher), secret_nonce = COALESCE($7, secret_nonce),
			updated_at = now()
		WHERE id = $1 AND account_id = $2`,
		itemID, accountID, strings.TrimSpace(in.Title), strings.TrimSpace(in.SiteURL),
		strings.TrimSpace(in.Username), ct, nonce,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Exists(ctx context.Context, accountID, itemID uuid.UUID) error {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vault_items WHERE id = $1 AND account_id = $2)`,
		itemID, accountID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, accountID, itemID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vault_items WHERE id = $1 AND account_id = $2`, itemID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func pick(override, fallback string) string {
	if v := strings.TrimSpace(override); v != "" {
		return v
	}
	return fallback
}
