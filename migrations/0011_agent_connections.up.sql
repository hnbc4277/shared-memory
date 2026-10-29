CREATE TABLE IF NOT EXISTS agent_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    agent TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL CHECK (kind IN ('auto', 'registered', 'device_linked')),
    device_id UUID REFERENCES devices (id) ON DELETE SET NULL,
    api_key_id UUID REFERENCES api_keys (id) ON DELETE SET NULL,
    last_ingest_at TIMESTAMPTZ,
    last_recall_at TIMESTAMPTZ,
    last_agent_path TEXT,
    last_model TEXT,
    last_external_session_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_connections_auto
    ON agent_connections (account_id, agent)
    WHERE kind = 'auto' AND revoked_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_connections_device
    ON agent_connections (device_id)
    WHERE device_id IS NOT NULL AND revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_connections_account_agent
    ON agent_connections (account_id, agent)
    WHERE revoked_at IS NULL;
