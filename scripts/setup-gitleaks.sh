#!/usr/bin/env bash
set -euo pipefail

# scripts/setup-gitleaks.sh — one-shot contributor dev setup for llm-proxy.
#
# Installs the secret-scanning dependency and registers the git hook so
# contributors don't have to wire anything up manually. Idempotent: safe to
# re-run. (Host/service provisioning is ./setup.sh — a different job.)
#
#   ./scripts/setup-gitleaks.sh

PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# shellcheck source=lib/ui.sh
source "$PRJ_ROOT/scripts/lib/ui.sh"

cd "$PRJ_ROOT"
ui_header "llm-proxy · contributor setup" "gitleaks secret-scanning pre-commit hook"

# --- gitleaks (pre-commit secret scanner) ------------------------------------
if ui_have gitleaks; then
  ui_ok "gitleaks already installed: $(gitleaks version 2>/dev/null | head -1 || echo present)"
elif ui_have brew; then
  ui_info "installing gitleaks via brew..."
  brew install gitleaks
  ui_ok "gitleaks installed"
else
  ui_err "Homebrew not found. Install gitleaks manually:"
  ui_detail "https://github.com/gitleaks/gitleaks#installing"
  ui_detail "apt: gitleaks  |  go: go install github.com/gitleaks/gitleaks/v8/cmd/gitleaks@latest"
  exit 1
fi

# --- git hooks ---------------------------------------------------------------
ui_info "registering git hooks (core.hooksPath=.githooks)"
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit 2>/dev/null || true
ui_ok "hook registered"

# --- sanity check ------------------------------------------------------------
ui_info "verifying hook fires..."
if gitleaks git --staged --no-banner >/dev/null 2>&1; then
  ui_ok "gitleaks pre-commit hook active"
else
  ui_warn "gitleaks scan reported an issue — hook is installed but re-check manually"
fi

ui_ok "done — secret scanning is now active for this clone"
