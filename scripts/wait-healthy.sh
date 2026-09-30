#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TIMEOUT="${WAIT_TIMEOUT:-180}"
SERVICES=(postgres redis minio opensearch qdrant)

deadline=$((SECONDS + TIMEOUT))
while (( SECONDS < deadline )); do
  all_ok=true
  for svc in "${SERVICES[@]}"; do
    cid="$(docker compose ps -q "$svc" 2>/dev/null || true)"
    if [[ -z "$cid" ]]; then
      all_ok=false
      break
    fi
    status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null || echo "unknown")"
    if [[ "$status" != "healthy" && "$status" != "running" ]]; then
      all_ok=false
      break
    fi
  done
  if $all_ok; then
    echo "All services ready."
    exit 0
  fi
  sleep 3
done

echo "Timed out after ${TIMEOUT}s waiting for services." >&2
docker compose ps >&2
exit 1
