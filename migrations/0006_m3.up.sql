ALTER TABLE sessions
    ADD COLUMN external_session_id TEXT;

CREATE UNIQUE INDEX idx_sessions_external
    ON sessions (account_id, agent, external_session_id)
    WHERE external_session_id IS NOT NULL;

CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    client_name TEXT NOT NULL,
    agent TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ,
    daemon_version TEXT,
    spool_pending INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_devices_account ON devices (account_id);

CREATE TABLE device_codes (
    device_code TEXT PRIMARY KEY,
    user_code TEXT NOT NULL UNIQUE,
    client_name TEXT NOT NULL,
    agent TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    account_id UUID REFERENCES accounts (id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices (id) ON DELETE SET NULL,
    issued_key TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    interval_seconds INT NOT NULL DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_device_codes_user ON device_codes (user_code);

ALTER TABLE api_keys
    ADD COLUMN device_id UUID REFERENCES devices (id) ON DELETE CASCADE;
