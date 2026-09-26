#!/usr/bin/env bash
# scripts/lib/ui.sh — shared terminal UI for the repo's dev scripts.
#
# Sourced, never executed:
#   source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/ui.sh"
#
# Bash 3.2+ (macOS /bin/bash), zero dependencies.
#
# Palette: Catppuccin Macchiato — the theme this repo's opencode TUI runs
# (see ~/.config/opencode/tui.json). Emitted as 24-bit truecolor when the
# terminal supports it, with a standard-ANSI fallback otherwise.
#
# Colour policy: ANSI is emitted only when stdout is a TTY, NO_COLOR is unset
# and TERM is not "dumb". Piped/CI output therefore stays clean, and callers
# that capture a script's output for a whiptail/dialog box should export
# NO_COLOR=1 so the captured log is plain text.
#
# See https://no-color.org for the NO_COLOR convention.

# shellcheck shell=bash

# Color capability. Catppuccin Macchiato is emitted as 24-bit truecolor when
# the terminal advertises it (COLORTERM=truecolor / 24bit, or a *-direct TERM),
# falling back to the nearest standard ANSI colours otherwise. Linux is the
# primary target; the fallback keeps older terminals readable.
if [[ -t 1 && -z "${NO_COLOR:-}" && "${TERM:-dumb}" != "dumb" ]]; then
  UI_BOLD=$'\033[1m'
  UI_NC=$'\033[0m'
  if [[ ${COLORTERM:-} == *truecolor* || ${COLORTERM:-} == *24bit* || ${TERM:-} == *-direct* ]]; then
    UI_TEXT=$'\033[38;2;202;211;245m'   # #cad3f5
    UI_SUB=$'\033[38;2;165;173;203m'    # #a5adcb subtext0
    UI_DIM=$'\033[38;2;110;115;141m'    # #6e738d overlay0
    UI_MAUVE=$'\033[38;2;198;160;246m'  # #c6a0f6
    UI_CYAN=$'\033[38;2;139;213;202m'   # #8bd5ca teal
    UI_GREEN=$'\033[38;2;166;218;149m'  # #a6da95
    UI_YELLOW=$'\033[38;2;238;212;159m' # #eed49f
    UI_RED=$'\033[38;2;237;135;150m'    # #ed8796
  else
    UI_TEXT=$'\033[37m';  UI_SUB=$'\033[37m';   UI_DIM=$'\033[90m'
    UI_MAUVE=$'\033[35m'; UI_CYAN=$'\033[36m'
    UI_GREEN=$'\033[32m'; UI_YELLOW=$'\033[33m'; UI_RED=$'\033[31m'
  fi
else
  UI_BOLD=; UI_NC=; UI_TEXT=; UI_SUB=; UI_DIM=; UI_MAUVE=
  UI_CYAN=; UI_GREEN=; UI_YELLOW=; UI_RED=
fi

# ── status lines ────────────────────────────────────────────────────────────
ui_info()   { printf '%s▌%s %s%s%s\n'   "$UI_MAUVE" "$UI_NC" "$UI_TEXT" "$1" "$UI_NC"; }
ui_ok()     { printf '  %s✓%s %s%s%s\n' "$UI_GREEN" "$UI_NC" "$UI_TEXT" "$1" "$UI_NC"; }
ui_warn()   { printf '  %s!%s %s%s%s\n' "$UI_YELLOW" "$UI_NC" "$UI_TEXT" "$1" "$UI_NC" >&2; }
ui_err()    { printf '  %s✗%s %s%s%s\n' "$UI_RED" "$UI_NC" "$UI_TEXT" "$1" "$UI_NC" >&2; }
ui_detail() { printf '    %s%s%s\n'     "$UI_DIM" "$1" "$UI_NC"; }
ui_die()    { ui_err "$1"; exit 1; }

# ── structure ───────────────────────────────────────────────────────────────
ui_have() { command -v "$1" >/dev/null 2>&1; }

ui_rule() { # $1 = optional width (default 54)
  local n="${1:-54}" line="" i
  for ((i = 0; i < n; i++)); do line="${line}─"; done
  printf '%s%s%s\n' "$UI_DIM" "$line" "$UI_NC"
}

ui_header() { # $1 = title, $2 = optional subtitle
  printf '\n%s╭─%s %s%s%s\n' "$UI_MAUVE" "$UI_NC" "$UI_BOLD$UI_TEXT" "$1" "$UI_NC"
  [[ -n "${2:-}" ]] && printf '%s│%s  %s%s%s\n' "$UI_MAUVE" "$UI_NC" "$UI_SUB" "$2" "$UI_NC"
  printf '%s╰%s' "$UI_MAUVE" "$UI_NC"
  ui_rule 54
}
