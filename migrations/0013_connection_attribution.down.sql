DROP INDEX IF EXISTS idx_recall_receipts_connection;
DROP INDEX IF EXISTS idx_memories_source_connection;
DROP INDEX IF EXISTS idx_messages_connection;

ALTER TABLE memories DROP COLUMN IF EXISTS source_connection_id;
ALTER TABLE messages DROP COLUMN IF EXISTS connection_id;
ALTER TABLE recall_receipts DROP COLUMN IF EXISTS connection_id;
