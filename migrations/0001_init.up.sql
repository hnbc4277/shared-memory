CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE account_policies (
    account_id UUID PRIMARY KEY REFERENCES accounts (id),
    store_transcript BOOLEAN NOT NULL DEFAULT TRUE,
    store_attachments BOOLEAN NOT NULL DEFAULT TRUE,
    inject_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    vault_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    artifact_searchable BOOLEAN NOT NULL DEFAULT TRUE,
    retention_days INT NOT NULL DEFAULT 0
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    agent TEXT NOT NULL,
    agent_path TEXT NOT NULL,
    title TEXT,
    status TEXT,
    started_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    transcript_object_key TEXT,
    transcript_hash TEXT,
    degraded BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions (id),
    account_id UUID NOT NULL REFERENCES accounts (id),
    role TEXT,
    seq INT,
    content_object_key TEXT,
    skills JSONB NOT NULL DEFAULT '[]',
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions (id),
    message_id UUID REFERENCES messages (id),
    account_id UUID NOT NULL REFERENCES accounts (id),
    kind TEXT,
    plugins JSONB NOT NULL DEFAULT '[]',
    tools JSONB NOT NULL DEFAULT '[]',
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    session_id UUID REFERENCES sessions (id),
    message_id UUID REFERENCES messages (id),
    agent_path TEXT,
    filename TEXT,
    mime TEXT,
    size_bytes BIGINT,
    content_hash TEXT,
    object_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    scope TEXT NOT NULL CHECK (scope IN ('global', 'project')),
    kind TEXT,
    body TEXT,
    source_session_id UUID REFERENCES sessions (id),
    source_agent_path TEXT,
    supersedes UUID REFERENCES memories (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    account_id UUID PRIMARY KEY REFERENCES accounts (id),
    body TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    action TEXT NOT NULL,
    meta JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_keys_account_id ON api_keys (account_id);
CREATE INDEX idx_projects_account_id ON projects (account_id);
CREATE INDEX idx_sessions_account_id ON sessions (account_id);
CREATE INDEX idx_sessions_project_id ON sessions (project_id);
CREATE INDEX idx_messages_session_id ON messages (session_id);
CREATE INDEX idx_messages_account_id ON messages (account_id);
CREATE INDEX idx_events_session_id ON events (session_id);
CREATE INDEX idx_events_account_id ON events (account_id);
CREATE INDEX idx_attachments_account_id ON attachments (account_id);
CREATE INDEX idx_memories_account_id ON memories (account_id);
CREATE INDEX idx_audit_logs_account_id ON audit_logs (account_id);
