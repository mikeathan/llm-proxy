#!/usr/bin/env bash
set -euo pipefail

# scripts/telegram-bot.sh — manage a Telegram webhook for a communication
# connector during development.
#
# Secrets come from the environment so they never live in this file:
#   TELEGRAM_BOT_TOKEN     bot token from @BotFather
#   LLM_PROXY_TUNNEL_URL   public base URL of your llm-proxy (e.g. ngrok)
#   TELEGRAM_SECRET_TOKEN  webhook secret_token (any random string)
#   TELEGRAM_CONNECTOR     connector name (default: telegram)
#
#   ./scripts/telegram-bot.sh register
#   ./scripts/telegram-bot.sh test "hello"
#   ./scripts/telegram-bot.sh delete

PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# shellcheck source=lib/ui.sh
source "$PRJ_ROOT/scripts/lib/ui.sh"

BOT_TOKEN="${TELEGRAM_BOT_TOKEN:-}"
TUNNEL_URL="${LLM_PROXY_TUNNEL_URL:-}"
SECRET_TOKEN="${TELEGRAM_SECRET_TOKEN:-}"
CONNECTOR_NAME="${TELEGRAM_CONNECTOR:-telegram}"

API="https://api.telegram.org/bot${BOT_TOKEN}"
WEBHOOK_URL="${TUNNEL_URL}/api/v1/webhooks/${CONNECTOR_NAME}"

usage() {
  cat <<'EOF'
Usage: scripts/telegram-bot.sh {register|test|delete} ["test message"]

Configure these environment variables first (never stored in the repo):
  TELEGRAM_BOT_TOKEN     bot token from @BotFather
  LLM_PROXY_TUNNEL_URL   public base URL, e.g. https://<id>.ngrok.app
  TELEGRAM_SECRET_TOKEN  webhook secret_token
  TELEGRAM_CONNECTOR     connector name (default: telegram)

Commands:
  register              Set the webhook and show its info
  test ["message"]      POST a fake update to the webhook (default: "test message")
  delete                Remove the webhook and show its info
EOF
}

require_config() {
  ui_have curl || ui_die "curl is required"
  ui_have jq || ui_die "jq is required (brew install jq | apt install jq)"
  [[ -n "$BOT_TOKEN" ]]    || ui_die "TELEGRAM_BOT_TOKEN is not set"
  [[ -n "$TUNNEL_URL" ]]   || ui_die "LLM_PROXY_TUNNEL_URL is not set"
  [[ -n "$SECRET_TOKEN" ]] || ui_die "TELEGRAM_SECRET_TOKEN is not set"
}

case "${1:-help}" in
  register)
    require_config
    ui_header "telegram webhook" "$WEBHOOK_URL"
    curl -sS -X POST "$API/setWebhook" \
      -d "url=$WEBHOOK_URL" -d "secret_token=$SECRET_TOKEN" | jq .
    ui_rule
    curl -sS "$API/getWebhookInfo" | jq .
    ;;
  test)
    require_config
    shift
    msg="${1:-test message}"
    ui_info "posting test update: $msg"
    curl -sS -X POST "$WEBHOOK_URL" \
      -H "Content-Type: application/json" \
      -H "X-Telegram-Bot-Api-Secret-Token: $SECRET_TOKEN" \
      -d "$(jq -n --arg msg "$msg" '{message:{text:$msg,chat:{id:1}}}')"
    echo
    ;;
  delete)
    require_config
    ui_header "telegram webhook" "delete"
    curl -sS -X POST "$API/deleteWebhook" | jq .
    ui_rule
    curl -sS "$API/getWebhookInfo" | jq .
    ;;
  -h | --help | help)
    usage
    ;;
  *)
    ui_err "unknown command: $1"
    usage
    exit 1
    ;;
esac
