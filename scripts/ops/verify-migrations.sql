-- Run against production Postgres (psql "$POSTGRES_DSN" -f scripts/ops/verify-migrations.sql)
SELECT version FROM schema_migrations ORDER BY version;

-- Required for agent connections + attribution (filename prefixes):
-- 0011_agent_connections, 0012_session_connection_id, 0013_connection_attribution
