#!/usr/bin/env bash
set -euo pipefail

LLM_URL="http://192.168.50.60:4001/api/conversation/message"
AUTH_URL="http://nodeherder.local:4110/api/auth/token"

CLIENT_ID="llm-proxy"
CLIENT_SECRET="${NODEHERDER_CLIENT_SECRET}"

CONVERSATION_ID="local-dev-1"
CONTEXT_VERSION="v1"

HISTORY_FILE="$HOME/.assistant_history"
touch "$HISTORY_FILE"

make_request() {
  curl -sS -w "\n%{http_code}" -X POST "$LLM_URL" \
    -H "Content-Type: application/json" \
    ${NODEHERDER_ACCESS_TOKEN:+-H "Authorization: Bearer $NODEHERDER_ACCESS_TOKEN"} \
    -d "{
      \"conversation_id\": \"$CONVERSATION_ID\",
      \"context_version\": \"$CONTEXT_VERSION\",
      \"message\": \"${USER_MESSAGE//\"/\\\"}\"
    }"
}

fetch_token() {
  echo "🔑 Fetching NodeHerder access token..."

  RESPONSE=$(curl -sS -X POST "$AUTH_URL" \
    -H "Content-Type: application/json" \
    -d "{
      \"client_id\": \"$CLIENT_ID\",
      \"client_secret\": \"$CLIENT_SECRET\"
    }")

  TOKEN=$(echo "$RESPONSE" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

  if [[ -z "$TOKEN" ]]; then
    echo "❌ Failed to obtain access token"
    return 1
  fi

  export NODEHERDER_ACCESS_TOKEN="$TOKEN"
  echo "✅ Token stored"
}

send_and_parse() {
  local RESPONSE
  RESPONSE=$(make_request)
  HTTP_STATUS="${RESPONSE##*$'\n'}"
  HTTP_BODY="${RESPONSE%$'\n'*}"
}

now_ms() {
  perl -MTime::HiRes=time -e 'printf "%.0f\n", time()*1000'
}

echo "🧠 Assistant CLI — type 'exit' to quit"

history -r "$HISTORY_FILE"

while true; do
  read -e -p "Message: " USER_MESSAGE
  [[ "$USER_MESSAGE" == "exit" ]] && exit 0
  [[ -z "$USER_MESSAGE" ]] && continue

  echo "$USER_MESSAGE" >> "$HISTORY_FILE"
  history -r "$HISTORY_FILE"

  echo "→ Sending request..."

  START_TIME=$(now_ms)

send_and_parse
BODY="$HTTP_BODY"
STATUS="$HTTP_STATUS"

if [[ "$STATUS" != "200" ]] && echo "$BODY" | grep -qi "get device context failed"; then
  fetch_token || continue
  send_and_parse
  BODY="$HTTP_BODY"
  STATUS="$HTTP_STATUS"
fi

END_TIME=$(now_ms)
ELAPSED_MS=$((END_TIME - START_TIME))

  echo
  echo "$BODY"
  echo "⏱  Time: ${ELAPSED_MS} ms"
done