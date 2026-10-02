DROP INDEX IF EXISTS idx_memories_supersedes;
DROP INDEX IF EXISTS idx_memories_account_project_kind;
DROP INDEX IF EXISTS idx_messages_account_agent_path;
DROP INDEX IF EXISTS idx_sessions_account_started;
DROP INDEX IF EXISTS idx_sessions_account_agent_path;
DROP INDEX IF EXISTS idx_messages_session_seq;

ALTER TABLE memories DROP COLUMN IF EXISTS updated_at;
ALTER TABLE memories DROP COLUMN IF EXISTS invalidated;

ALTER TABLE attachments DROP COLUMN IF EXISTS binary_stored;
ALTER TABLE attachments DROP COLUMN IF EXISTS derived_text;

ALTER TABLE messages DROP COLUMN IF EXISTS attachment_ids;
ALTER TABLE messages DROP COLUMN IF EXISTS content_hash;
ALTER TABLE messages DROP COLUMN IF EXISTS agent_path;
ALTER TABLE messages DROP COLUMN IF EXISTS agent;
