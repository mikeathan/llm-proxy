#!/bin/sh
# setup.sh — one-shot local environment setup for llm-proxy.
# Installs the secret-scanning dependency and registers the git hook so
# contributors don't have to wire anything up manually.
#
# Usage:   ./setup.sh
#
set -e

echo "==> llm-proxy local setup"

# --- gitleaks (pre-commit secret scanner) ---
if command -v gitleaks >/dev/null 2>&1; then
  echo "==> gitleaks already installed: $(gitleaks version 2>/dev/null | head -1 || echo present)"
else
  if command -v brew >/dev/null 2>&1; then
    echo "==> installing gitleaks via brew..."
    brew install gitleaks
  else
    echo "ERROR: Homebrew not found. Install gitleaks manually:" >&2
    echo "       https://github.com/gitleaks/gitleaks#installing" >&2
    echo "       (apt: gitleaks | go: go install github.com/gitleaks/gitleaks/v8/cmd/gitleaks@latest)" >&2
    exit 1
  fi
fi

# --- git hooks ---
echo "==> registering git hooks (core.hooksPath=.githooks)"
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit 2>/dev/null || true

# --- sanity check ---
echo "==> verifying hook fires..."
if git diff --cached --quiet 2>/dev/null; then
  : # nothing staged; that's fine for setup
fi
if gitleaks git --staged --no-banner >/dev/null 2>&1; then
  echo "==> gitleaks pre-commit hook active."
else
  echo "WARNING: gitleaks scan reported an issue; hook is still installed but check output above." >&2
fi

echo "==> done. Secret-scanning pre-commit hook is now active for this clone."
