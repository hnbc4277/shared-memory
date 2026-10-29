ALTER TABLE agent_connections
    ADD COLUMN IF NOT EXISTS setup_script TEXT;
