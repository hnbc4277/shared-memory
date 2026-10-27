ALTER TABLE agent_connections
    ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ;
