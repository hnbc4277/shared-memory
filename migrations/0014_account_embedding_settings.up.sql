CREATE TABLE account_embedding_settings (
    account_id UUID PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    protocol TEXT NOT NULL DEFAULT 'tei',
    base_url TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    dim INT NOT NULL DEFAULT 0,
    api_key_ciphertext BYTEA,
    api_key_nonce BYTEA,
    active_fingerprint TEXT,
    reindex_required BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_embedding_protocol_chk CHECK (protocol IN ('tei', 'openai'))
);
