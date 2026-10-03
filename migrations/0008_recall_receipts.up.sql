CREATE TABLE IF NOT EXISTS recall_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects (id) ON DELETE SET NULL,
    session_id UUID REFERENCES sessions (id) ON DELETE SET NULL,
    external_session_id TEXT,
    agent TEXT,
    agent_path TEXT,
    queried_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    hit_count INT NOT NULL DEFAULT 0,
    degraded BOOLEAN NOT NULL DEFAULT FALSE,
    warnings JSONB NOT NULL DEFAULT '[]',
    source TEXT NOT NULL DEFAULT 'http',
    query_preview TEXT
);
CREATE INDEX IF NOT EXISTS idx_recall_receipts_account_time
    ON recall_receipts (account_id, queried_at DESC);
CREATE INDEX IF NOT EXISTS idx_recall_receipts_ext
    ON recall_receipts (account_id, external_session_id, queried_at DESC)
    WHERE external_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_recall_receipts_session
    ON recall_receipts (account_id, session_id, queried_at DESC)
    WHERE session_id IS NOT NULL;
