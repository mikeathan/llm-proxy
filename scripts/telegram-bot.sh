#!/usr/bin/env bash
set -euo pipefail

# ── Edit these for your bot ──────────────────────────────
BOT_TOKEN="<your-telegram-bot-token>"
TUNNEL_URL="<your-tunnel-url>"
SECRET_TOKEN="<your-secret-token>"
CONNECTOR_NAME="<your-connector-name>"
# ─────────────────────────────────────────────────────────

API="https://api.telegram.org/bot$BOT_TOKEN"
WEBHOOK_URL="$TUNNEL_URL/api/v1/webhooks/$CONNECTOR_NAME"

case "${1:-help}" in
  register)
    curl -s -X POST "$API/setWebhook" \
      -d "url=$WEBHOOK_URL" -d "secret_token=$SECRET_TOKEN" | jq .
    echo "---"
    curl -s "$API/getWebhookInfo" | jq . ;;
  test)
    shift
    msg="${1:-test message}"
    curl -s -X POST "$WEBHOOK_URL" \
      -H "Content-Type: application/json" \
      -H "X-Telegram-Bot-Api-Secret-Token: $SECRET_TOKEN" \
      -d "$(jq -n --arg msg "$msg" '{message:{text:$msg,chat:{id:1}}}')" ;;
  delete)
    curl -s -X POST "$API/deleteWebhook" | jq .
    echo "---"
    curl -s "$API/getWebhookInfo" | jq . ;;
  *)
    echo "Usage: $0 {register|test|delete} [test message]"
    echo ""
    echo "First edit the 4 variables at the top of this script."
    echo ""
    echo "Commands:"
    echo "  register            Set webhook + show info"
    echo "  test [\"message\"]    POST to webhook (default: \"test message\")"
    echo "  delete              Remove webhook + show info" ;;
esac
