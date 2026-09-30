#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TIMEOUT="${WAIT_TIMEOUT:-180}"
SERVICES=(postgres redis minio opensearch qdrant)

service_has_healthcheck() {
  local cid="$1"
  docker inspect -f '{{if .Config.Healthcheck}}true{{else}}false{{end}}' "$cid" 2>/dev/null | grep -q true
}

deadline=$((SECONDS + TIMEOUT))
while (( SECONDS < deadline )); do
  all_ok=true
  for svc in "${SERVICES[@]}"; do
    cid="$(docker compose ps -q "$svc" 2>/dev/null || true)"
    if [[ -z "$cid" ]]; then
      all_ok=false
      break
    fi
    if service_has_healthcheck "$cid"; then
      status="$(docker inspect -f '{{.State.Health.Status}}' "$cid" 2>/dev/null || echo "unknown")"
      if [[ "$status" != "healthy" ]]; then
        all_ok=false
        break
      fi
    else
      status="$(docker inspect -f '{{.State.Status}}' "$cid" 2>/dev/null || echo "unknown")"
      if [[ "$status" != "running" ]]; then
        all_ok=false
        break
      fi
    fi
  done
  if $all_ok; then
    for svc in "${SERVICES[@]}"; do
      cid="$(docker compose ps -q "$svc")"
      if ! service_has_healthcheck "$cid"; then
        echo "WARN: service ${svc} has no healthcheck; accepting running but readiness is not health-gated" >&2
      fi
    done
    echo "All services ready."
    exit 0
  fi
  sleep 3
done

echo "Timed out after ${TIMEOUT}s waiting for services." >&2
docker compose ps >&2
exit 1
