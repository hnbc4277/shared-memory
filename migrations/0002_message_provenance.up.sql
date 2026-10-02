ALTER TABLE messages ADD COLUMN agent TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN agent_path TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN content_hash TEXT;
ALTER TABLE messages ADD COLUMN attachment_ids JSONB NOT NULL DEFAULT '[]';

ALTER TABLE attachments ADD COLUMN derived_text TEXT;
ALTER TABLE attachments ADD COLUMN binary_stored BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE memories ADD COLUMN invalidated BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE memories ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX idx_messages_session_seq ON messages (session_id, seq);
CREATE INDEX idx_sessions_account_agent_path ON sessions (account_id, agent_path);
CREATE INDEX idx_sessions_account_started ON sessions (account_id, started_at DESC);
CREATE INDEX idx_messages_account_agent_path ON messages (account_id, agent_path);
CREATE INDEX idx_memories_account_project_kind ON memories (account_id, project_id, kind);
CREATE INDEX idx_memories_supersedes ON memories (supersedes);
