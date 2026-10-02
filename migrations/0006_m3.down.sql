ALTER TABLE api_keys DROP COLUMN IF EXISTS device_id;
DROP TABLE IF EXISTS device_codes;
DROP TABLE IF EXISTS devices;
DROP INDEX IF EXISTS idx_sessions_external;
ALTER TABLE sessions DROP COLUMN IF EXISTS external_session_id;
