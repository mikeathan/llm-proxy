#!/usr/bin/env bash
# Print the memory scoreboard for one or more run directories.
# Usage: scripts/memory-scoreboard.sh <run-dir>...
#   e.g. scripts/memory-scoreboard.sh ~/.config/llm-proxy/runs/<ws>/<model>/<task>/<run>
# Columns the run files cannot supply print n/a. See
# docs/PLANS/memory/small-context-memory.md (Phase 0).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Resolve relative run dirs before changing directory.
args=()
for d in "$@"; do args+=("$(cd "$d" && pwd)"); done
cd "$root/backend"
exec go run ./tools/memory-scoreboard/ "${args[@]}"
