#!/usr/bin/env sh
# Shared Memory connection keepalive (install-side).
# Reads SHARED_MEMORY_API_URL / SHARED_MEMORY_API_KEY / SHARED_MEMORY_CONNECTION_ID.
# Optional SHARED_MEMORY_HEARTBEAT_INTERVAL (seconds, default 30).
set -eu
API_URL="${SHARED_MEMORY_API_URL:-http://127.0.0.1:18765}"
API_KEY="${SHARED_MEMORY_API_KEY:-}"
CONN_ID="${SHARED_MEMORY_CONNECTION_ID:-}"
INTERVAL="${SHARED_MEMORY_HEARTBEAT_INTERVAL:-30}"

if [ -z "$API_KEY" ]; then
  echo "sm-connection-heartbeat: SHARED_MEMORY_API_KEY is required" >&2
  exit 1
fi

API_URL="${API_URL%/}"

post() {
  if [ -n "$CONN_ID" ]; then
    body="{\"client\":\"setup\",\"connection_id\":\"${CONN_ID}\"}"
  else
    body='{"client":"setup"}'
  fi
  curl -sS -o /dev/null \
    -X POST "${API_URL}/v1/agents/connections/heartbeat" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$body" || echo "sm-connection-heartbeat: request failed" >&2
}

while true; do
  post
  sleep "$INTERVAL"
done
