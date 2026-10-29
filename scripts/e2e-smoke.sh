#!/usr/bin/env bash
# Full black-box smoke (parity target: e2e-smoke.ps1). Linux/macOS server.
set -euo pipefail
API="${API:-http://127.0.0.1:18080}"
MARKER="smoke-$(date +%s)"
WORKER_WAIT="${WORKER_WAIT_SECONDS:-45}"
KB_WAIT="${KB_WAIT_SECONDS:-45}"

pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*" >&2; exit 1; }

need_parser() {
  command -v jq >/dev/null 2>&1 || command -v python3 >/dev/null 2>&1 || fail "need jq or python3"
}

jget() {
  local expr=$1
  if command -v jq >/dev/null 2>&1; then
    jq -r "$expr"
  else
    python3 -c "import sys,json; d=json.load(sys.stdin); print($expr)"
  fi
}

curl_sm() {
  local method=$1 path=$2 body=${3:-} key=${4:-}
  local args=(-sS -w "\n%{http_code}" -X "$method" "$API$path" -H "Content-Type: application/json; charset=utf-8")
  [[ -n "$key" ]] && args+=(-H "Authorization: Bearer $key")
  [[ -n "$body" ]] && args+=(-d "$body")
  local out
  out=$(curl "${args[@]}")
  HTTP_CODE=$(echo "$out" | tail -n1)
  BODY=$(echo "$out" | sed '$d')
}

json_content() {
  python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'
}

new_account() {
  local email="smoke-$RANDOM-$RANDOM@example.com"
  curl_sm POST /v1/auth/register "{\"email\":\"$email\",\"password\":\"password123456\"}" ""
  [[ "$HTTP_CODE" == "201" ]] || fail "register HTTP $HTTP_CODE"
  echo "$BODY" | jget '.api_key'
}

new_message() {
  local project=$1 role=$2 content=$3
  local c
  c=$(echo "$content" | json_content)
  printf '{"project_id":"%s","agent":"xiaolongxia","agent_path":"xiaolongxia/researcher","skills":["deep-research"],"plugins":["browser"],"tools":["web_fetch"],"model":"deepseek-v3","role":"%s","content":%s}' \
    "$project" "$role" "$c"
}

need_parser
echo "Smoke → $API"
curl -sf "$API/healthz" >/dev/null || fail "healthz"
pass "healthz"

A=$(new_account)
B=$(new_account)
pass "two accounts"

curl_sm POST /v1/projects '{"name":"smoke"}' "$A"
[[ "$HTTP_CODE" == "201" ]] || fail "create project"
PROJECT=$(echo "$BODY" | jget '.id')

MSG=$(new_message "$PROJECT" user "我们决定采用 OpenSearch 做全文检索 $MARKER")
curl_sm POST /v1/ingest "$MSG" "$A"
[[ "$HTTP_CODE" == "201" ]] || fail "ingest marker"
pass "ingest"

curl_sm POST /v1/agents/connections '{"agent":"cursor","label":"bash-ded","issue_dedicated_key":true}' "$A"
DED_KEY=$(echo "$BODY" | jget '.api_key')

curl_sm POST /v1/agents/connections '{"agent":"xiaolongxia","label":"bash-claim","issue_dedicated_key":false}' "$A"
REG_ID=$(echo "$BODY" | jget '.connection_id')
curl_sm POST /v1/ingest "$(new_message "$PROJECT" user "claim $MARKER")" "$A"
curl_sm POST "/v1/agents/connections/$REG_ID/claim-history" "{}" "$A"
echo "$BODY" | grep -q sessions_claimed && pass "claim-history"

ISO_B="iso-B-$MARKER"
curl_sm POST /v1/agents/connections '{"agent":"cursor","label":"iso-b","issue_dedicated_key":true}' "$A"
KEY_B_ISO=$(echo "$BODY" | jget '.api_key')
curl_sm POST /v1/ingest "{\"agent\":\"cursor\",\"agent_path\":\"cursor\",\"role\":\"user\",\"content\":\"$ISO_B\"}" "$KEY_B_ISO"
leaked=1
for ((i=0; i<WORKER_WAIT; i+=2)); do
  curl_sm POST /v1/recall "{\"query\":\"$ISO_B\",\"top_k\":10}" "$DED_KEY"
  echo "$BODY" | grep -q "$ISO_B" || { leaked=0; break; }
  sleep 2
done
[[ "$leaked" -eq 0 ]] || fail "recall isolation"
pass "recall isolation"

hit=0
for ((i=0; i<WORKER_WAIT; i++)); do
  curl_sm POST /v1/recall "{\"project_id\":\"$PROJECT\",\"query\":\"$MARKER OpenSearch\"}" "$A"
  echo "$BODY" | grep -q "$MARKER" && { hit=1; break; }
  sleep 1
done
[[ "$hit" -eq 1 ]] || fail "memory recall"
pass "memory recall"
curl_sm POST /v1/recall "{\"query\":\"$MARKER OpenSearch\"}" "$B"
echo "$BODY" | grep -q "$MARKER" && fail "cross-account recall"
pass "memory cross-account"

curl_sm PUT /v1/policy '{"store_transcript":false}' "$A"
curl_sm POST /v1/ingest "$(new_message "$PROJECT" user "no transcript")" "$A"
SID=$(echo "$BODY" | jget '.session_id')
curl_sm GET "/v1/sessions/$SID" "" "$A"
echo "$BODY" | grep -q '"content_stored":false' || fail "policy"
pass "policy transcript off"
curl_sm PUT /v1/policy '{"store_transcript":true}' "$A"

SECRET="Zx9!$(openssl rand -hex 3 2>/dev/null || echo abc123)"
curl_sm POST /v1/ingest "$(new_message "$PROJECT" user "GitLab https://gitlab.example.com 账号: alice 密码: $SECRET")" "$A"
CAND=$(echo "$BODY" | jget '.prompts[] | select(.kind=="vault") | .candidate_id' | head -1)
[[ -n "$CAND" && "$CAND" != "null" ]] || fail "vault prompt"
curl_sm POST "/v1/vault/candidates/$CAND/accept" "{}" "$A"
ITEM=$(echo "$BODY" | jget '.id')
curl_sm POST "/v1/vault/items/$ITEM/reveal" "{\"password\":\"password123456\"}" "$A"
echo "$BODY" | grep -q "$SECRET" || fail "vault reveal"
pass "vault"
curl_sm POST /v1/recall "{\"project_id\":\"$PROJECT\",\"query\":\"GitLab 密码\"}" "$A"
echo "$BODY" | grep -q "$SECRET" && fail "vault leak"
pass "vault recall clean"

LONG=$(python3 -c "print('# note\n' + '消费者组。' * 60)")
ARTMSG=$(printf '{"project_id":"%s","agent":"xiaolongxia","agent_path":"xiaolongxia/researcher","role":"assistant","content":%s}' "$PROJECT" "$(echo "$LONG" | json_content)")
curl_sm POST /v1/ingest "$ARTMSG" "$A"
ACAND=$(echo "$BODY" | jget '.prompts[] | select(.kind=="artifact") | .candidate_id' | head -1)
[[ -n "$ACAND" ]] || fail "artifact prompt"
curl_sm POST "/v1/artifacts/candidates/$ACAND/accept" "{}" "$A"
AID=$(echo "$BODY" | jget '.artifact.id')
curl_sm GET "/v1/artifacts/$AID" "" "$A"
echo "$BODY" | grep -q "消费者组" || fail "artifact"
pass "artifact"

KB_MARKER="smkb$MARKER"
TMP=$(mktemp)
echo "# KB $KB_MARKER account_id" > "$TMP"
HTTP_CODE=$(curl -sS -o /tmp/kb-up.json -w "%{http_code}" -X POST "$API/v1/kb/documents" \
  -H "Authorization: Bearer $A" -F "title=Smoke KB" -F "file=@$TMP;filename=spec.md")
rm -f "$TMP"
[[ "$HTTP_CODE" == "201" ]] || fail "kb upload"
DOC_ID=$(jget '.document.id' < /tmp/kb-up.json)
ready=0
for ((i=0; i<KB_WAIT; i++)); do
  curl_sm GET "/v1/kb/documents/$DOC_ID" "" "$A"
  ST=$(echo "$BODY" | jget '.document.status')
  [[ "$ST" == "ready" ]] && { ready=1; break; }
  [[ "$ST" == "failed" ]] && fail "kb failed"
  sleep 1
done
[[ "$ready" -eq 1 ]] || fail "kb ready"
pass "kb ready"
for ((i=0; i<15; i++)); do
  curl_sm POST /v1/recall "{\"query\":\"$KB_MARKER account_id\"}" "$A"
  echo "$BODY" | grep -q "$KB_MARKER" && break
  sleep 1
done
echo "$BODY" | grep -q kb_hits || fail "kb_hits"
pass "kb recall"
curl_sm GET "/v1/kb/documents/$DOC_ID" "" "$B"
[[ "$HTTP_CODE" == "404" ]] || fail "kb cross-account"

DEL_CODE=$(curl -sS -o /dev/null -w "%{http_code}" -X DELETE "$API/v1/account" \
  -H "Authorization: Bearer $A" -H "X-Confirm: delete")
[[ "$DEL_CODE" == "204" ]] || fail "delete account"
curl_sm GET /v1/sessions "" "$A"
[[ "$HTTP_CODE" == "401" ]] || fail "A revoked"
curl_sm GET /v1/sessions "" "$B"
[[ "$HTTP_CODE" == "200" ]] || fail "B ok"
pass "account delete"

echo "All full bash smoke checks passed."
