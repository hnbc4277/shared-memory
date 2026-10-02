package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/objectstore"
)

var ErrNotFound = errors.New("artifact not found")

const (
	ConsentPrompt = "prompt"
	ConsentManual = "manual"
)

type Candidate struct {
	ID        uuid.UUID  `json:"id"`
	ProjectID *uuid.UUID `json:"project_id"`
	SessionID *uuid.UUID `json:"session_id"`
	AgentPath string     `json:"agent_path"`
	Title     string     `json:"title"`
	Type      string     `json:"type"`
	Summary   string     `json:"summary"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

type Artifact struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       *uuid.UUID `json:"project_id"`
	Title           string     `json:"title"`
	Type            string     `json:"type"`
	Summary         string     `json:"summary"`
	Tags            []string   `json:"tags"`
	CurrentVersion  int        `json:"current_version"`
	SourceSessionID *uuid.UUID `json:"source_session_id"`
	SourceAgentPath string     `json:"source_agent_path"`
	ConsentMethod   string     `json:"consent_method"`
	ConsentedAt     time.Time  `json:"consented_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Version struct {
	Version   int       `json:"version"`
	BodyHash  string    `json:"body_hash"`
	CreatedAt time.Time `json:"created_at"`
}

type Detail struct {
	Artifact Artifact  `json:"artifact"`
	Body     string    `json:"body"`
	Versions []Version `json:"versions"`
}

type Source struct {
	ProjectID *uuid.UUID
	SessionID *uuid.UUID
	MessageID *uuid.UUID
	AgentPath string
}

type Input struct {
	ProjectID *uuid.UUID `json:"project_id"`
	Title     string     `json:"title"`
	Type      string     `json:"type"`
	Tags      []string   `json:"tags"`
	Body      string     `json:"body"`
}

type Filter struct {
	ProjectID *uuid.UUID
	Type      string
	Tag       string
}

type Store struct {
	pool    *pgxpool.Pool
	objects objectstore.Store
}

func NewStore(pool *pgxpool.Pool, objects objectstore.Store) *Store {
	return &Store{pool: pool, objects: objects}
}

func (s *Store) putBody(ctx context.Context, key, body string) (string, error) {
	sum := sha256.Sum256([]byte(body))
	if err := s.objects.Put(ctx, key, strings.NewReader(body), int64(len(body)), "text/markdown; charset=utf-8"); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) readBody(ctx context.Context, key string) (string, error) {
	rc, err := s.objects.Get(ctx, key)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// CreateCandidate stores the generated content pending the user's decision.
func (s *Store) CreateCandidate(ctx context.Context, accountID uuid.UUID, src Source, body string) (Candidate, error) {
	c := Candidate{ID: uuid.New(), ProjectID: src.ProjectID, SessionID: src.SessionID, AgentPath: src.AgentPath,
		Title: GuessTitle(body), Type: GuessType(body), Summary: Summarize(body)}
	key := objectstore.ArtifactCandidateKey(accountID, src.ProjectID, c.ID)
	if _, err := s.putBody(ctx, key, body); err != nil {
		return Candidate{}, err
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO artifact_candidates (id, account_id, project_id, session_id, message_id, agent_path,
			title, type, summary, body_object_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING created_at, expires_at`,
		c.ID, accountID, src.ProjectID, src.SessionID, src.MessageID, src.AgentPath, c.Title, c.Type, c.Summary, key,
	).Scan(&c.CreatedAt, &c.ExpiresAt)
	if err != nil {
		_ = s.objects.Delete(ctx, key)
		return Candidate{}, fmt.Errorf("insert artifact candidate: %w", err)
	}
	return c, nil
}

func (s *Store) ListCandidates(ctx context.Context, accountID uuid.UUID) ([]Candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, session_id, agent_path, title, type, summary, created_at, expires_at
		FROM artifact_candidates WHERE account_id = $1 AND expires_at > now() ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.SessionID, &c.AgentPath, &c.Title, &c.Type, &c.Summary,
			&c.CreatedAt, &c.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type AcceptOverrides struct {
	Title string   `json:"title"`
	Type  string   `json:"type"`
	Tags  []string `json:"tags"`
}

func (s *Store) Accept(ctx context.Context, accountID, candidateID uuid.UUID, o AcceptOverrides) (Detail, error) {
	var (
		projectID, sessionID *uuid.UUID
		agentPath, title     string
		typ, key             string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT project_id, session_id, agent_path, title, type, body_object_key
		FROM artifact_candidates WHERE id = $1 AND account_id = $2 AND expires_at > now()`,
		candidateID, accountID,
	).Scan(&projectID, &sessionID, &agentPath, &title, &typ, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	body, err := s.readBody(ctx, key)
	if err != nil {
		return Detail{}, fmt.Errorf("read candidate body: %w", err)
	}
	if v := strings.TrimSpace(o.Title); v != "" {
		title = v
	}
	if ValidType(o.Type) {
		typ = o.Type
	}
	d, err := s.create(ctx, accountID, Input{ProjectID: projectID, Title: title, Type: typ, Tags: o.Tags, Body: body},
		Source{ProjectID: projectID, SessionID: sessionID, AgentPath: agentPath}, ConsentPrompt)
	if err != nil {
		return Detail{}, err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM artifact_candidates WHERE id = $1`, candidateID); err != nil {
		return Detail{}, err
	}
	_ = s.objects.Delete(ctx, key)
	return d, nil
}

func (s *Store) Reject(ctx context.Context, accountID, candidateID uuid.UUID) error {
	var key string
	err := s.pool.QueryRow(ctx,
		`DELETE FROM artifact_candidates WHERE id = $1 AND account_id = $2 RETURNING body_object_key`,
		candidateID, accountID,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_ = s.objects.Delete(ctx, key)
	return nil
}

// Create saves an artifact the user explicitly asked to keep (manual consent).
func (s *Store) Create(ctx context.Context, accountID uuid.UUID, in Input, src Source) (Detail, error) {
	return s.create(ctx, accountID, in, src, ConsentManual)
}

func (s *Store) create(ctx context.Context, accountID uuid.UUID, in Input, src Source, consent string) (Detail, error) {
	if strings.TrimSpace(in.Body) == "" {
		return Detail{}, fmt.Errorf("body is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = GuessTitle(in.Body)
	}
	if !ValidType(in.Type) {
		in.Type = GuessType(in.Body)
	}
	tags := cleanTags(in.Tags)
	a := Artifact{ID: uuid.New(), ProjectID: in.ProjectID, Title: strings.TrimSpace(in.Title), Type: in.Type,
		Summary: Summarize(in.Body), Tags: tags, CurrentVersion: 1, SourceSessionID: src.SessionID,
		SourceAgentPath: src.AgentPath, ConsentMethod: consent}
	key := objectstore.ArtifactVersionKey(accountID, in.ProjectID, a.ID, 1)
	hash, err := s.putBody(ctx, key, in.Body)
	if err != nil {
		return Detail{}, err
	}
	tagsJSON, _ := json.Marshal(tags)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		INSERT INTO artifacts (id, account_id, project_id, title, type, summary, tags, source_session_id,
			source_agent_path, consent_method)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING consented_at, created_at, updated_at`,
		a.ID, accountID, a.ProjectID, a.Title, a.Type, a.Summary, tagsJSON, a.SourceSessionID, a.SourceAgentPath, consent,
	).Scan(&a.ConsentedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Detail{}, fmt.Errorf("insert artifact: %w", err)
	}
	var v Version
	err = tx.QueryRow(ctx, `
		INSERT INTO artifact_versions (artifact_id, account_id, version, body_object_key, body_hash)
		VALUES ($1,$2,1,$3,$4) RETURNING version, body_hash, created_at`,
		a.ID, accountID, key, hash,
	).Scan(&v.Version, &v.BodyHash, &v.CreatedAt)
	if err != nil {
		return Detail{}, fmt.Errorf("insert artifact version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return Detail{Artifact: a, Body: in.Body, Versions: []Version{v}}, nil
}

func (s *Store) List(ctx context.Context, accountID uuid.UUID, f Filter) ([]Artifact, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, title, type, summary, tags, current_version, source_session_id, source_agent_path,
		       consent_method, consented_at, created_at, updated_at
		FROM artifacts
		WHERE account_id = $1
		  AND ($2::uuid IS NULL OR project_id = $2)
		  AND ($3 = '' OR type = $3)
		  AND ($4 = '' OR tags ? $4)
		ORDER BY updated_at DESC`,
		accountID, f.ProjectID, f.Type, f.Tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanArtifact(row pgx.Row) (Artifact, error) {
	var a Artifact
	var tags []byte
	err := row.Scan(&a.ID, &a.ProjectID, &a.Title, &a.Type, &a.Summary, &tags, &a.CurrentVersion,
		&a.SourceSessionID, &a.SourceAgentPath, &a.ConsentMethod, &a.ConsentedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Artifact{}, err
	}
	a.Tags = []string{}
	_ = json.Unmarshal(tags, &a.Tags)
	return a, nil
}

func (s *Store) Get(ctx context.Context, accountID, id uuid.UUID) (Detail, error) {
	a, err := scanArtifact(s.pool.QueryRow(ctx, `
		SELECT id, project_id, title, type, summary, tags, current_version, source_session_id, source_agent_path,
		       consent_method, consented_at, created_at, updated_at
		FROM artifacts WHERE id = $1 AND account_id = $2`, id, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT version, body_hash, created_at, body_object_key FROM artifact_versions
		WHERE artifact_id = $1 AND account_id = $2 ORDER BY version DESC`, id, accountID)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()
	d := Detail{Artifact: a, Versions: []Version{}}
	currentKey := ""
	for rows.Next() {
		var v Version
		var key string
		if err := rows.Scan(&v.Version, &v.BodyHash, &v.CreatedAt, &key); err != nil {
			return Detail{}, err
		}
		if v.Version == a.CurrentVersion {
			currentKey = key
		}
		d.Versions = append(d.Versions, v)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, err
	}
	if currentKey != "" {
		if d.Body, err = s.readBody(ctx, currentKey); err != nil {
			return Detail{}, fmt.Errorf("read artifact body: %w", err)
		}
	}
	return d, nil
}

type Update struct {
	Title *string   `json:"title"`
	Type  *string   `json:"type"`
	Tags  *[]string `json:"tags"`
	Body  *string   `json:"body"`
}

// Update edits metadata; a changed body becomes a new version.
func (s *Store) Update(ctx context.Context, accountID, id uuid.UUID, u Update) (Detail, error) {
	cur, err := s.Get(ctx, accountID, id)
	if err != nil {
		return Detail{}, err
	}
	a := cur.Artifact
	if u.Title != nil && strings.TrimSpace(*u.Title) != "" {
		a.Title = strings.TrimSpace(*u.Title)
	}
	if u.Type != nil && ValidType(*u.Type) {
		a.Type = *u.Type
	}
	if u.Tags != nil {
		a.Tags = cleanTags(*u.Tags)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	if u.Body != nil && *u.Body != cur.Body && strings.TrimSpace(*u.Body) != "" {
		a.CurrentVersion++
		a.Summary = Summarize(*u.Body)
		key := objectstore.ArtifactVersionKey(accountID, a.ProjectID, a.ID, a.CurrentVersion)
		hash, err := s.putBody(ctx, key, *u.Body)
		if err != nil {
			return Detail{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO artifact_versions (artifact_id, account_id, version, body_object_key, body_hash)
			VALUES ($1,$2,$3,$4,$5)`, a.ID, accountID, a.CurrentVersion, key, hash); err != nil {
			return Detail{}, err
		}
	}
	tagsJSON, _ := json.Marshal(a.Tags)
	if _, err := tx.Exec(ctx, `
		UPDATE artifacts SET title=$3, type=$4, tags=$5, summary=$6, current_version=$7, updated_at=now()
		WHERE id=$1 AND account_id=$2`,
		a.ID, accountID, a.Title, a.Type, tagsJSON, a.Summary, a.CurrentVersion); err != nil {
		return Detail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return s.Get(ctx, accountID, id)
}

func (s *Store) Delete(ctx context.Context, accountID, id uuid.UUID) error {
	var projectID *uuid.UUID
	err := s.pool.QueryRow(ctx,
		`DELETE FROM artifacts WHERE id = $1 AND account_id = $2 RETURNING project_id`, id, accountID,
	).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	prefix := strings.TrimSuffix(objectstore.ArtifactVersionKey(accountID, projectID, id, 0), "v0000.md")
	return s.objects.DeletePrefix(ctx, prefix)
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
