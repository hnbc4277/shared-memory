ALTER TABLE account_policies
    ADD COLUMN IF NOT EXISTS kb_inject_enabled BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE recall_receipts
    ADD COLUMN IF NOT EXISTS kb_hit_count INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS kb_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    mime TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL CHECK (source_type IN ('upload', 'artifact', 'attachment')),
    source_artifact_id UUID REFERENCES artifacts (id) ON DELETE SET NULL,
    source_attachment_id UUID REFERENCES attachments (id) ON DELETE SET NULL,
    object_key TEXT NOT NULL DEFAULT '',
    bytes BIGINT NOT NULL DEFAULT 0,
    content_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'ready', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_kb_documents_account_time
    ON kb_documents (account_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_documents_account_hash_ready
    ON kb_documents (account_id, content_hash)
    WHERE status = 'ready' AND content_hash <> '';

CREATE TABLE IF NOT EXISTS kb_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES kb_documents (id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    ordinal INT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    char_start INT,
    char_end INT,
    UNIQUE (document_id, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_kb_chunks_account_doc
    ON kb_chunks (account_id, document_id);
