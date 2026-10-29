#!/usr/bin/env bash
# Example: daily Postgres backup (cron). Set POSTGRES_DSN or PG* vars.
set -euo pipefail
OUT_DIR="${BACKUP_DIR:-./backups}"
mkdir -p "$OUT_DIR"
STAMP=$(date +%Y%m%d-%H%M%S)
FILE="$OUT_DIR/sharedmemory-$STAMP.sql.gz"
if [[ -n "${POSTGRES_DSN:-}" ]]; then
  pg_dump "$POSTGRES_DSN" | gzip > "$FILE"
else
  pg_dump -U "${PGUSER:-sm}" -h "${PGHOST:-127.0.0.1}" -p "${PGPORT:-5432}" "${PGDATABASE:-sharedmemory}" | gzip > "$FILE"
fi
echo "Wrote $FILE"
find "$OUT_DIR" -name 'sharedmemory-*.sql.gz' -mtime +14 -delete 2>/dev/null || true
