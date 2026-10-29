DROP INDEX IF EXISTS idx_sessions_account_connection;
ALTER TABLE sessions DROP COLUMN IF EXISTS connection_id;
