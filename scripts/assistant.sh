#!/usr/bin/env bash
set -euo pipefail

# scripts/assistant.sh — small interactive CLI client for the conversation
# message endpoint, for poking the backend without the web UI. It keeps a
# readline history file and auto-refreshes the NodeHerder token when the
# backend reports stale device context.
#
# Env overrides:
#   LLM_PROXY_URL              backend base URL (default http://localhost:4001)
#   CONVERSATION_ID            conversation to continue (default local-dev-1)
#   CONTEXT_VERSION            context version (default v1)
#   ASSISTANT_HISTORY_FILE     readline history file (default ~/.assistant_history)
#   NODEHERDER_AUTH_URL        token endpoint
#   NODEHERDER_CLIENT_ID       auth client id (default llm-proxy)
#   NODEHERDER_CLIENT_SECRET   auth client secret
#   NODEHERDER_ACCESS_TOKEN    pre-fetched bearer token (optional)
#
# Requires: curl, perl, jq.

PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# shellcheck source=lib/ui.sh
source "$PRJ_ROOT/scripts/lib/ui.sh"

ui_have curl || ui_die "curl is required"
ui_have jq || ui_die "jq is required (brew install jq | apt install jq)"

LLM_PROXY_URL="${LLM_PROXY_URL:-http://localhost:4001}"
LLM_URL="$LLM_PROXY_URL/api/conversation/message"
AUTH_URL="${NODEHERDER_AUTH_URL:-http://nodeherder.local:4110/api/auth/token}"
CLIENT_ID="${NODEHERDER_CLIENT_ID:-llm-proxy}"
CLIENT_SECRET="${NODEHERDER_CLIENT_SECRET:-}"
CONVERSATION_ID="${CONVERSATION_ID:-local-dev-1}"
CONTEXT_VERSION="${CONTEXT_VERSION:-v1}"
HISTORY_FILE="${ASSISTANT_HISTORY_FILE:-$HOME/.assistant_history}"
touch "$HISTORY_FILE"

HTTP_STATUS=""
HTTP_BODY=""

now_ms() { perl -MTime::HiRes=time -e 'printf "%.0f\n", time()*1000'; }

make_request() {
  local body
  body="$(jq -n \
    --arg cid "$CONVERSATION_ID" \
    --arg cv "$CONTEXT_VERSION" \
    --arg msg "$USER_MESSAGE" \
    '{conversation_id: $cid, context_version: $cv, message: $msg}')"
  if [[ -n "${NODEHERDER_ACCESS_TOKEN:-}" ]]; then
    curl -sS -w $'\n%{http_code}' -X POST "$LLM_URL" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer ${NODEHERDER_ACCESS_TOKEN}" -d "$body"
  else
    curl -sS -w $'\n%{http_code}' -X POST "$LLM_URL" \
      -H "Content-Type: application/json" -d "$body"
  fi
}

fetch_token() {
  ui_info "fetching NodeHerder access token..."
  [[ -n "$CLIENT_SECRET" ]] || ui_die "NODEHERDER_CLIENT_SECRET is not set (needed for token refresh)"
  local resp token
  resp="$(jq -n --arg cid "$CLIENT_ID" --arg sec "$CLIENT_SECRET" \
    '{client_id: $cid, client_secret: $sec}' | \
    curl -sS -X POST "$AUTH_URL" -H "Content-Type: application/json" -d @-)"
  token="$(printf '%s' "$resp" | jq -r '.access_token // empty' 2>/dev/null || true)"
  [[ -n "$token" ]] || { ui_err "failed to obtain access token"; return 1; }
  export NODEHERDER_ACCESS_TOKEN="$token"
  ui_ok "token stored"
}

send_and_parse() {
  local resp
  resp="$(make_request)"
  HTTP_STATUS="${resp##*$'\n'}"
  HTTP_BODY="${resp%$'\n'*}"
}

trap 'echo; ui_info "bye"; exit 0' INT
ui_header "llm-proxy assistant" "$LLM_URL"
ui_detail "conversation: $CONVERSATION_ID · type 'exit' to quit"
history -r "$HISTORY_FILE" 2>/dev/null || true

while true; do
  printf '%s▸%s ' "$UI_CYAN$UI_BOLD" "$UI_NC"
  read -r -e -p "Message: " USER_MESSAGE || { echo; ui_info "bye"; exit 0; }
  [[ "$USER_MESSAGE" == "exit" ]] && { ui_info "bye"; exit 0; }
  [[ -z "$USER_MESSAGE" ]] && continue

  printf '%s\n' "$USER_MESSAGE" >> "$HISTORY_FILE"
  history -r "$HISTORY_FILE" 2>/dev/null || true

  ui_detail "sending request..."
  START_TIME="$(now_ms)"

  send_and_parse
  if [[ "$HTTP_STATUS" != "200" ]] && grep -qi "get device context failed" <<<"$HTTP_BODY"; then
    fetch_token || continue
    send_and_parse
  fi

  ELAPSED_MS=$(( $(now_ms) - START_TIME ))
  echo
  printf '%s\n' "$HTTP_BODY"
  ui_detail "HTTP $HTTP_STATUS · ${ELAPSED_MS} ms"
done
