DROP TABLE IF EXISTS web_sessions;
DROP TABLE IF EXISTS entity_aliases;
DROP TABLE IF EXISTS tool_calls;
DROP TABLE IF EXISTS ingest_quarantine;
ALTER TABLE attachments DROP COLUMN IF EXISTS thumbnail_object_key;
ALTER TABLE accounts
    DROP COLUMN IF EXISTS vault_salt,
    DROP COLUMN IF EXISTS vault_wrapped_dek,
    DROP COLUMN IF EXISTS vault_wrap_nonce;
