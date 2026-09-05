#!/usr/bin/env bash
# launch.sh — run llm-proxy directly (foreground, no systemd) for development.
#
# State root resolution, in order:
#   1. anything you export before calling (LLM_PROXY_HOME, --data ...) wins —
#      the env file only supplies defaults, never clobbers
#   2. .launch.env (generated/refreshed by setup.sh install) — mirrors the
#      installed service's data root so dev and service agree on paths
#   3. the app's implicit fallback (~/.config/llm-proxy)
#
# Usage: ./launch.sh [extra args passed through to the binary]
#        LLM_PROXY_HOME=~/dev-state ./launch --data ~/other
set -euo pipefail

PRJ_ROOT="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="$PRJ_ROOT/.launch.env"
BIN="$PRJ_ROOT/backend/llm-proxy"

# Defaults from the last install — but only for variables the caller has NOT
# already set (":=" assignment), so explicit exports always win.
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  source "$ENV_FILE"
fi
: "${LLM_PROXY_HOME:=}"

# Build if the binary is missing (first run / fresh clone).
if [[ ! -x "$BIN" ]]; then
  echo "launch.sh: binary not built — running scripts/build.sh first..." >&2
  BUILD_INLINE=1 bash "$PRJ_ROOT/scripts/build.sh"
fi

# The frontend assets are embedded at build time; warn (don't block) if they
# were never compiled — the UI will serve nothing.
if [[ ! -d "$PRJ_ROOT/backend/frontend_dist" ]]; then
  echo "launch.sh: warning — backend/frontend_dist missing; run: (cd frontend && npm run build) then rebuild" >&2
fi

if [[ -n "$LLM_PROXY_HOME" ]]; then
  echo "launch.sh: state root $LLM_PROXY_HOME (from ${ENV_FILE#$PRJ_ROOT/}; export LLM_PROXY_HOME to override)" >&2
else
  echo "launch.sh: no LLM_PROXY_HOME set — using the app default (~/.config/llm-proxy)" >&2
fi

exec env ${LLM_PROXY_HOME:+LLM_PROXY_HOME="$LLM_PROXY_HOME"} "$BIN" "$@"
