ALTER TABLE sessions
    ADD COLUMN connection_id UUID REFERENCES agent_connections (id) ON DELETE SET NULL;

CREATE INDEX idx_sessions_account_connection
    ON sessions (account_id, connection_id)
    WHERE connection_id IS NOT NULL;
