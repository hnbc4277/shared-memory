DROP TABLE IF EXISTS kb_chunks;
DROP TABLE IF EXISTS kb_documents;
ALTER TABLE recall_receipts DROP COLUMN IF EXISTS kb_hit_count;
ALTER TABLE account_policies DROP COLUMN IF EXISTS kb_inject_enabled;
