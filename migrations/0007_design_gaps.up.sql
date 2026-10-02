-- Thumbnails, quarantine, tool calls, entity aliases, web sessions, vault password wrap.

ALTER TABLE attachments
    ADD COLUMN IF NOT EXISTS thumbnail_object_key TEXT;

CREATE TABLE IF NOT EXISTS ingest_quarantine (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID REFERENCES accounts (id) ON DELETE CASCADE,
    agent TEXT,
    agent_path TEXT,
    reason TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_quarantine_account ON ingest_quarantine (account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS tool_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    message_id UUID REFERENCES messages (id) ON DELETE SET NULL,
    kind TEXT NOT NULL CHECK (kind IN ('plugin', 'tool')),
    name TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    output_truncated TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tool_calls_session ON tool_calls (session_id, started_at);

CREATE TABLE IF NOT EXISTS entity_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects (id) ON DELETE CASCADE,
    canonical TEXT NOT NULL,
    alias TEXT NOT NULL,
    source_memory_id UUID REFERENCES memories (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_entity_aliases_unique
    ON entity_aliases (account_id, COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid), alias);
CREATE INDEX IF NOT EXISTS idx_entity_aliases_canonical ON entity_aliases (account_id, canonical);

CREATE TABLE IF NOT EXISTS web_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_web_sessions_account ON web_sessions (account_id);

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS vault_salt BYTEA,
    ADD COLUMN IF NOT EXISTS vault_wrapped_dek BYTEA,
    ADD COLUMN IF NOT EXISTS vault_wrap_nonce BYTEA;
