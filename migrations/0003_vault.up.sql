CREATE TABLE vault_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    session_id UUID REFERENCES sessions (id),
    message_id UUID REFERENCES messages (id),
    agent_path TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    site_url TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    secret_cipher BYTEA NOT NULL,
    secret_nonce BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '7 days'
);

CREATE TABLE vault_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id),
    project_id UUID REFERENCES projects (id),
    title TEXT NOT NULL DEFAULT '',
    site_url TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    secret_cipher BYTEA NOT NULL,
    secret_nonce BYTEA NOT NULL,
    source_session_id UUID REFERENCES sessions (id),
    source_agent_path TEXT NOT NULL DEFAULT '',
    consent_method TEXT NOT NULL,
    consented_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vault_candidates_account ON vault_candidates (account_id);
CREATE INDEX idx_vault_items_account ON vault_items (account_id);
