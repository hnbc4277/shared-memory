CREATE TABLE artifact_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    session_id UUID REFERENCES sessions (id),
    message_id UUID REFERENCES messages (id),
    agent_path TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    type TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    body_object_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '7 days'
);

CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    title TEXT NOT NULL,
    type TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '[]',
    current_version INT NOT NULL DEFAULT 1,
    source_session_id UUID REFERENCES sessions (id),
    source_agent_path TEXT NOT NULL DEFAULT '',
    consent_method TEXT NOT NULL,
    consented_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE artifact_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id UUID NOT NULL REFERENCES artifacts (id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts (id),
    version INT NOT NULL,
    body_object_key TEXT NOT NULL,
    body_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, version)
);

CREATE INDEX idx_artifact_candidates_account ON artifact_candidates (account_id);
CREATE INDEX idx_artifacts_account ON artifacts (account_id, updated_at DESC);
