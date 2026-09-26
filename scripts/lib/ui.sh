#!/usr/bin/env bash
# scripts/lib/ui.sh — shared terminal UI for the repo's dev scripts.
#
# Sourced, never executed:
#   source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/ui.sh"
#
# Bash 3.2+ (macOS /bin/bash), zero dependencies.
#
# Colour policy: ANSI is emitted only when stdout is a TTY, NO_COLOR is unset
# and TERM is not "dumb". Piped/CI output therefore stays clean, and callers
# that capture a script's output for a whiptail/dialog box should export
# NO_COLOR=1 so the captured log is plain text.
#
# See https://no-color.org for the NO_COLOR convention.

# shellcheck shell=bash

if [[ -t 1 && -z "${NO_COLOR:-}" && "${TERM:-dumb}" != "dumb" ]]; then
  UI_BOLD=$'\033[1m';    UI_DIM=$'\033[2m'
  UI_RED=$'\033[0;31m';  UI_GREEN=$'\033[0;32m'; UI_YELLOW=$'\033[0;33m'
  UI_CYAN=$'\033[0;36m'
  UI_NC=$'\033[0m'
else
  UI_BOLD=; UI_DIM=; UI_RED=; UI_GREEN=; UI_YELLOW=; UI_CYAN=; UI_NC=
fi

# ── status lines ────────────────────────────────────────────────────────────
ui_info()   { printf '%s▸%s %s\n'    "$UI_BOLD" "$UI_NC" "$1"; }
ui_ok()     { printf '  %s✓%s %s\n'  "$UI_GREEN" "$UI_NC" "$1"; }
ui_warn()   { printf '  %s!%s %s\n'  "$UI_YELLOW" "$UI_NC" "$1" >&2; }
ui_err()    { printf '  %s✗%s %s\n'  "$UI_RED" "$UI_NC" "$1" >&2; }
ui_detail() { printf '    %s%s%s\n'  "$UI_DIM" "$1" "$UI_NC"; }
ui_die()    { ui_err "$1"; exit 1; }

# ── structure ───────────────────────────────────────────────────────────────
ui_have() { command -v "$1" >/dev/null 2>&1; }

ui_rule() { # $1 = optional width (default 54)
  local n="${1:-54}" line="" i
  for ((i = 0; i < n; i++)); do line="${line}─"; done
  printf '%s%s%s\n' "$UI_DIM" "$line" "$UI_NC"
}

ui_header() { # $1 = title, $2 = optional subtitle
  printf '\n%s%s%s\n' "$UI_CYAN$UI_BOLD" "$1" "$UI_NC"
  [[ -n "${2:-}" ]] && printf '%s%s%s\n' "$UI_DIM" "$2" "$UI_NC"
  ui_rule 54
}
