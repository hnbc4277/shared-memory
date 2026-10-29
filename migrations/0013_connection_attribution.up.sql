ALTER TABLE recall_receipts
    ADD COLUMN IF NOT EXISTS connection_id UUID REFERENCES agent_connections (id) ON DELETE SET NULL;

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS connection_id UUID REFERENCES agent_connections (id) ON DELETE SET NULL;

ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS source_connection_id UUID REFERENCES agent_connections (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_messages_connection
    ON messages (account_id, connection_id)
    WHERE connection_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_memories_source_connection
    ON memories (account_id, source_connection_id)
    WHERE source_connection_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_recall_receipts_connection
    ON recall_receipts (account_id, connection_id, queried_at DESC)
    WHERE connection_id IS NOT NULL;
