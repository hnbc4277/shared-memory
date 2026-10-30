package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"sharedmemory/internal/auth"
)

var (
	ErrNotFound    = errors.New("connection not found")
	ErrInvalid     = errors.New("invalid connection")
	ErrBadAgent    = errors.New("agent is required")
)

type Store struct {
	pool *pgxpool.Pool
	auth *auth.Store
}

func NewStore(pool *pgxpool.Pool, authStore *auth.Store) *Store {
	return &Store{pool: pool, auth: authStore}
}

const connSelect = `c.id, c.agent, COALESCE(c.label,''), c.kind, c.device_id, c.api_key_id,
	c.last_ingest_at, c.last_recall_at, c.last_heartbeat_at, COALESCE(c.last_agent_path,''), COALESCE(c.last_model,''),
	COALESCE(c.last_external_session_id,''), c.created_at,
	(c.setup_script IS NOT NULL AND length(c.setup_script) > 0)`

const connReturning = `id, agent, COALESCE(label,''), kind, device_id, api_key_id,
	last_ingest_at, last_recall_at, COALESCE(last_agent_path,''), COALESCE(last_model,''),
	COALESCE(last_external_session_id,''), created_at`

func (s *Store) TouchHeartbeat(ctx context.Context, accountID, connectionID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE agent_connections
		SET last_heartbeat_at = now(), updated_at = now()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
		  AND (last_heartbeat_at IS NULL OR last_heartbeat_at < now() - interval '5 seconds')`,
		connectionID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if err := s.ValidateConnection(ctx, accountID, connectionID); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (s *Store) ValidateConnection(ctx context.Context, accountID, connectionID uuid.UUID) error {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM agent_connections
			WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
		)`, connectionID, accountID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecordIngest(ctx context.Context, accountID uuid.UUID, connectionID *uuid.UUID, act Activity) error {
	agent := strings.TrimSpace(act.Agent)
	if agent == "" {
		return nil
	}
	if connectionID != nil {
		return s.touchConnection(ctx, accountID, *connectionID, true, act)
	}
	return s.touchAuto(ctx, accountID, agent, true, act)
}

func (s *Store) RecordRecall(ctx context.Context, accountID uuid.UUID, connectionID *uuid.UUID, act Activity) error {
	agent := strings.TrimSpace(act.Agent)
	if connectionID != nil {
		if err := s.touchConnection(ctx, accountID, *connectionID, false, act); err != nil {
			return err
		}
		return nil
	}
	if agent == "" {
		return nil
	}
	return s.touchAuto(ctx, accountID, agent, false, act)
}

func (s *Store) touchAuto(ctx context.Context, accountID uuid.UUID, agent string, ingest bool, act Activity) error {
	var n int64
	if ingest {
		tag, err := s.pool.Exec(ctx, `
			UPDATE agent_connections SET
				last_ingest_at = now(),
				last_agent_path = COALESCE(NULLIF($4,''), last_agent_path),
				last_model = COALESCE(NULLIF($5,''), last_model),
				last_external_session_id = COALESCE(NULLIF($6,''), last_external_session_id),
				updated_at = now()
			WHERE account_id = $1 AND agent = $2 AND kind = $3 AND revoked_at IS NULL`,
			accountID, agent, KindAuto, act.AgentPath, act.Model, act.ExternalSessionID)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
	} else {
		tag, err := s.pool.Exec(ctx, `
			UPDATE agent_connections SET
				last_recall_at = now(),
				last_agent_path = COALESCE(NULLIF($4,''), last_agent_path),
				last_model = COALESCE(NULLIF($5,''), last_model),
				last_external_session_id = COALESCE(NULLIF($6,''), last_external_session_id),
				updated_at = now()
			WHERE account_id = $1 AND agent = $2 AND kind = $3 AND revoked_at IS NULL`,
			accountID, agent, KindAuto, act.AgentPath, act.Model, act.ExternalSessionID)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
	}
	if n == 0 {
		if ingest {
			_, err := s.pool.Exec(ctx, `
				INSERT INTO agent_connections (account_id, agent, label, kind, last_ingest_at,
					last_agent_path, last_model, last_external_session_id, updated_at)
				VALUES ($1,$2,$3,$4, now(), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), now())`,
				accountID, agent, DefaultAutoLabel, KindAuto, act.AgentPath, act.Model, act.ExternalSessionID)
			return err
		}
		_, err := s.pool.Exec(ctx, `
			INSERT INTO agent_connections (account_id, agent, label, kind, last_recall_at,
				last_agent_path, last_model, last_external_session_id, updated_at)
			VALUES ($1,$2,$3,$4, now(), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), now())`,
			accountID, agent, DefaultAutoLabel, KindAuto, act.AgentPath, act.Model, act.ExternalSessionID)
		return err
	}
	return nil
}

func (s *Store) touchConnection(ctx context.Context, accountID, id uuid.UUID, ingest bool, act Activity) error {
	if ingest {
		tag, err := s.pool.Exec(ctx, `
			UPDATE agent_connections SET
				last_ingest_at = now(),
				last_agent_path = COALESCE(NULLIF($3,''), last_agent_path),
				last_model = COALESCE(NULLIF($4,''), last_model),
				last_external_session_id = COALESCE(NULLIF($5,''), last_external_session_id),
				updated_at = now()
			WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`,
			id, accountID, act.AgentPath, act.Model, act.ExternalSessionID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE agent_connections SET
			last_recall_at = now(),
			last_agent_path = COALESCE(NULLIF($3,''), last_agent_path),
			last_model = COALESCE(NULLIF($4,''), last_model),
			last_external_session_id = COALESCE(NULLIF($5,''), last_external_session_id),
			updated_at = now()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`,
		id, accountID, act.AgentPath, act.Model, act.ExternalSessionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SyncDeviceLinks(ctx context.Context, accountID uuid.UUID) error {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, COALESCE(d.agent,''), COALESCE(d.client_name,'')
		FROM devices d
		WHERE d.account_id = $1 AND d.revoked_at IS NULL`, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID uuid.UUID
		var agent, clientName string
		if err := rows.Scan(&deviceID, &agent, &clientName); err != nil {
			return err
		}
		if agent == "" {
			agent = "device"
		}
		label := strings.TrimSpace(clientName)
		if label == "" {
			label = "Device"
		}
		_, err := s.pool.Exec(ctx, `
			INSERT INTO agent_connections (account_id, agent, label, kind, device_id, updated_at)
			SELECT $1,$2,$3,$4,$5, now()
			WHERE NOT EXISTS (
				SELECT 1 FROM agent_connections
				WHERE device_id = $5 AND revoked_at IS NULL
			)`, accountID, agent, label, KindDeviceLinked, deviceID)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) ListGrouped(ctx context.Context, accountID uuid.UUID) ([]Group, error) {
	if err := s.SyncDeviceLinks(ctx, accountID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rows, err := s.pool.Query(ctx, `
		SELECT `+connSelect+`,
			d.last_seen_at, COALESCE(d.client_name,''), COALESCE(d.daemon_version,''), COALESCE(d.spool_pending,0),
			COALESCE(k.name,'')
		FROM agent_connections c
		LEFT JOIN devices d ON d.id = c.device_id AND d.revoked_at IS NULL
		LEFT JOIN api_keys k ON k.id = c.api_key_id
		WHERE c.account_id = $1 AND c.revoked_at IS NULL
		ORDER BY c.agent, c.kind, c.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byAgent := map[string]*Group{}
	order := []string{}
	for rows.Next() {
		var c Connection
		var deviceID, keyID *uuid.UUID
		var connHB, deviceHB *time.Time
		var clientName, daemonVer, keyName string
		var spool int
		err := rows.Scan(&c.ID, &c.Agent, &c.Label, &c.Kind, &deviceID, &keyID,
			&c.LastIngestAt, &c.LastRecallAt, &connHB, &c.LastAgentPath, &c.LastModel, &c.LastExternalSessionID, &c.CreatedAt,
			&c.HasSetupScript,
			&deviceHB, &clientName, &daemonVer, &spool, &keyName)
		if err != nil {
			return nil, err
		}
		c.DeviceID = deviceID
		eff := LatestActivity(connHB, deviceHB, nil)
		c.LastHeartbeatAt = eff
		if clientName != "" && c.Kind == KindDeviceLinked {
			if c.Label == "" || c.Label == "Device" {
				c.Label = clientName
			}
			c.ClientName = clientName
		}
		c.DaemonVersion = daemonVer
		c.SpoolPending = spool
		c.APIKeyName = keyName
		c.ProcessOnline = ProcessOnline(now, eff)
		c.ActivityFresh = ActivityFresh(now, c.LastIngestAt, c.LastRecallAt)
		c.Online = c.ProcessOnline || c.ActivityFresh

		g, ok := byAgent[c.Agent]
		if !ok {
			g = &Group{Agent: c.Agent, DisplayName: DisplayName(c.Agent)}
			byAgent[c.Agent] = g
			order = append(order, c.Agent)
		}
		g.Instances = append(g.Instances, c)
		g.InstanceCount++
		if c.Online {
			g.OnlineCount++
		}
		act := LatestActivity(eff, c.LastIngestAt, c.LastRecallAt)
		if act != nil && (g.LastActivityAt == nil || act.After(*g.LastActivityAt)) {
			g.LastActivityAt = act
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(order))
	for _, a := range order {
		out = append(out, *byAgent[a])
	}
	return out, nil
}

func (s *Store) CreateRegistered(ctx context.Context, accountID uuid.UUID, in CreateInput) (CreateResult, error) {
	agent := strings.TrimSpace(in.Agent)
	if agent == "" {
		return CreateResult{}, ErrBadAgent
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = DisplayName(agent) + " 连接"
	}
	id := uuid.New()
	var keyID *uuid.UUID
	var plaintext string
	if in.IssueDedicatedKey && s.auth != nil {
		var kid uuid.UUID
		var err error
		plaintext, kid, err = s.auth.CreateAPIKeyRecord(ctx, accountID, "conn:"+label, nil)
		if err != nil {
			return CreateResult{}, err
		}
		keyID = &kid
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO agent_connections (id, account_id, agent, label, kind, api_key_id)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING `+connReturning, id, accountID, agent, label, KindRegistered, keyID)
	c, err := scanConnection(row)
	if err != nil {
		return CreateResult{}, err
	}
	c.Online = false
	return CreateResult{Connection: c, APIKey: plaintext}, nil
}

// ConnectionIDForAPIKey returns the registered connection bound to a dedicated API key, if any.
func (s *Store) ConnectionIDForAPIKey(ctx context.Context, accountID, apiKeyID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM agent_connections
		WHERE account_id = $1 AND api_key_id = $2 AND revoked_at IS NULL`, accountID, apiKeyID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// DedicatedKeyLink maps a dedicated API key to its agent connection.
type DedicatedKeyLink struct {
	ConnectionID uuid.UUID
	Label        string
}

// DedicatedKeyLinks returns api_key_id → connection for all non-revoked dedicated keys on the account.
func (s *Store) DedicatedKeyLinks(ctx context.Context, accountID uuid.UUID) (map[uuid.UUID]DedicatedKeyLink, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT api_key_id, id, COALESCE(label,'')
		FROM agent_connections
		WHERE account_id = $1 AND revoked_at IS NULL AND api_key_id IS NOT NULL`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]DedicatedKeyLink{}
	for rows.Next() {
		var keyID, connID uuid.UUID
		var label string
		if err := rows.Scan(&keyID, &connID, &label); err != nil {
			return nil, err
		}
		out[keyID] = DedicatedKeyLink{ConnectionID: connID, Label: label}
	}
	return out, rows.Err()
}

func (s *Store) PatchLabel(ctx context.Context, accountID, id uuid.UUID, label string) (Connection, error) {
	label = strings.TrimSpace(label)
	row := s.pool.QueryRow(ctx, `
		UPDATE agent_connections SET label = $3, updated_at = now()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
		RETURNING `+connReturning, id, accountID, label)
	c, err := scanConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
	 return Connection{}, ErrNotFound
	}
	return c, err
}

// ClaimHistory assigns unattributed sessions (connection_id IS NULL) for agent to a registered connection.
func (s *Store) ClaimHistory(ctx context.Context, accountID, connectionID uuid.UUID, before *time.Time) (int64, error) {
	var agent, kind string
	err := s.pool.QueryRow(ctx, `
		SELECT agent, kind FROM agent_connections
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`, connectionID, accountID).Scan(&agent, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if kind != KindRegistered {
		return 0, ErrInvalid
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions SET connection_id = $1
		WHERE account_id = $2 AND agent = $3 AND connection_id IS NULL
		  AND ($4::timestamptz IS NULL OR started_at <= $4)`,
		connectionID, accountID, agent, before)
	if err != nil {
		return 0, err
	}
	n := tag.RowsAffected()
	if n == 0 {
		return 0, nil
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE messages m SET connection_id = s.connection_id
		FROM sessions s
		WHERE m.session_id = s.id AND s.account_id = $2 AND s.connection_id = $1
		  AND m.connection_id IS NULL`, connectionID, accountID); err != nil {
		return n, err
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE memories m SET source_connection_id = s.connection_id
		FROM sessions s
		WHERE m.source_session_id = s.id AND s.account_id = $2 AND s.connection_id = $1
		  AND m.source_connection_id IS NULL`, connectionID, accountID); err != nil {
		return n, err
	}
	return n, nil
}

// UpgradeAuto revokes an auto row and creates a registered connection for the same agent.
func (s *Store) UpgradeAuto(ctx context.Context, accountID, autoID uuid.UUID, label string, issueKey, claimHistory bool) (CreateResult, error) {
	var kind, agent string
	err := s.pool.QueryRow(ctx, `
		SELECT kind, agent FROM agent_connections
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`, autoID, accountID).Scan(&kind, &agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreateResult{}, ErrNotFound
	}
	if err != nil {
		return CreateResult{}, err
	}
	if kind != KindAuto {
		return CreateResult{}, ErrInvalid
	}
	created, err := s.CreateRegistered(ctx, accountID, CreateInput{
		Agent: agent, Label: label, IssueDedicatedKey: issueKey,
	})
	if err != nil {
		return CreateResult{}, err
	}
	_, _ = s.pool.Exec(ctx, `
		UPDATE agent_connections r SET
			last_ingest_at = COALESCE(r.last_ingest_at, a.last_ingest_at),
			last_recall_at = COALESCE(r.last_recall_at, a.last_recall_at),
			last_agent_path = COALESCE(NULLIF(r.last_agent_path,''), a.last_agent_path),
			last_model = COALESCE(NULLIF(r.last_model,''), a.last_model),
			last_external_session_id = COALESCE(NULLIF(r.last_external_session_id,''), a.last_external_session_id),
			updated_at = now()
		FROM agent_connections a
		WHERE r.id = $1 AND a.id = $2 AND r.account_id = $3`, created.Connection.ID, autoID, accountID)
	if err := s.Revoke(ctx, accountID, autoID); err != nil {
		return CreateResult{}, err
	}
	if claimHistory {
		if _, err := s.ClaimHistory(ctx, accountID, created.Connection.ID, nil); err != nil {
			return CreateResult{}, err
		}
	}
	got, err := s.ListGrouped(ctx, accountID)
	if err == nil {
		for _, g := range got {
			for _, inst := range g.Instances {
				if inst.ID == created.Connection.ID {
					created.Connection = inst
					break
				}
			}
		}
	}
	return created, nil
}

func (s *Store) Revoke(ctx context.Context, accountID, id uuid.UUID) error {
	return s.RevokeMaybeKey(ctx, accountID, id, false)
}

// RevokeMaybeKey marks a connection revoked; when revokeDedicatedKey is true and api_key_id is set, revokes that key too.
func (s *Store) RevokeMaybeKey(ctx context.Context, accountID, id uuid.UUID, revokeDedicatedKey bool) error {
	var keyID *uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT api_key_id FROM agent_connections
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`, id, accountID).Scan(&keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE agent_connections SET revoked_at = now(), updated_at = now()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`, id, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if revokeDedicatedKey && keyID != nil && s.auth != nil {
		_ = s.auth.RevokeAPIKey(ctx, accountID, *keyID)
	}
	return nil
}

func (s *Store) RevokeByDevice(ctx context.Context, accountID, deviceID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE agent_connections SET revoked_at = now(), updated_at = now()
		WHERE account_id = $1 AND device_id = $2 AND revoked_at IS NULL`, accountID, deviceID)
	return err
}

// SaveSetupScript stores a personalized client setup script (may contain the dedicated API key).
func (s *Store) SaveSetupScript(ctx context.Context, accountID, connectionID uuid.UUID, script string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE agent_connections SET setup_script = $3, updated_at = now()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`, connectionID, accountID, script)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSetupScript returns the stored setup script, connection label, and agent for download.
func (s *Store) GetSetupScript(ctx context.Context, accountID, connectionID uuid.UUID) (script, label, agent string, err error) {
	var raw *string
	err = s.pool.QueryRow(ctx, `
		SELECT setup_script, COALESCE(label,''), agent
		FROM agent_connections
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`, connectionID, accountID).Scan(&raw, &label, &agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return "", "", "", ErrNotFound
	}
	return *raw, label, agent, nil
}

func scanConnection(row pgx.Row) (Connection, error) {
	var c Connection
	var deviceID, keyID *uuid.UUID
	err := row.Scan(&c.ID, &c.Agent, &c.Label, &c.Kind, &deviceID, &keyID,
		&c.LastIngestAt, &c.LastRecallAt, &c.LastAgentPath, &c.LastModel, &c.LastExternalSessionID, &c.CreatedAt)
	c.DeviceID = deviceID
	return c, err
}

func ParseConnectionID(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, fmt.Errorf("empty")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, ErrInvalid
	}
	return id, nil
}
