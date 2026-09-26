#!/usr/bin/env bash
set -euo pipefail

# scripts/build.sh — compile the llm-proxy binary (frontend assets + backend).
#
# Single build implementation: setup.sh and launch.sh both call this script, and
# it runs standalone for manual builds.
#
# Callers that render its output inside their own UI (whiptail/dialog panels)
# set BUILD_INLINE=1 for lean output and NO_COLOR=1 so the captured log is plain
# text — matching the two modes:
#   ./scripts/build.sh                     # pretty standalone build
#   BUILD_INLINE=1 NO_COLOR=1 ./scripts/build.sh   # lean, plain, for a TUI box

PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_NAME="llm-proxy"
INLINE=${BUILD_INLINE:-0}
START_SECONDS=$SECONDS

# shellcheck source=lib/ui.sh
source "$PRJ_ROOT/scripts/lib/ui.sh"
# shellcheck source=lib/toolchain.sh
source "$PRJ_ROOT/scripts/lib/toolchain.sh"

cd "$PRJ_ROOT"

if [[ $INLINE != 1 ]]; then
  ui_header "llm-proxy · build" "frontend assets + backend binary"
fi

# --- Repo ownership guard ----------------------------------------------------
# Root-owned files in the tree (typically left behind by an earlier root-run
# build) make vite/npm fail mid-build with EACCES. Fail fast with the fix, or
# self-heal when the script itself runs as root.
if find . -user root -print -quit 2>/dev/null | grep -q .; then
  if [[ $EUID -eq 0 && -n "${SUDO_USER:-}" ]]; then
    ui_info "Repairing root-owned files (from an earlier root-run build)..."
    chown -R "${SUDO_USER}":"$(id -gn "$SUDO_USER")" "$PRJ_ROOT"
    ui_ok "repo ownership repaired"
  else
    ui_err "Found root-owned files in the repo (vite will fail with EACCES)."
    ui_detail "Fix:  sudo chown -R \"$(id -un)\":\"$(id -gn)\" $PRJ_ROOT"
    exit 1
  fi
fi

# --- Versioning (from the most recent git tag) -------------------------------
ui_info "Retrieving version from Git tags..."
VERSION=$(git tag --sort=-v:refname | head -n 1)
[[ -n "$VERSION" ]] || VERSION="dev"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ui_ok "Version ${UI_GREEN}${VERSION}${UI_NC} (commit ${UI_CYAN}${COMMIT}${UI_NC})"

# --- Frontend build ----------------------------------------------------------
ui_info "Building frontend assets..."
if (cd frontend && npm install && npm run build); then
  ui_ok "Frontend generated successfully."
else
  ui_die "Frontend build failed!"
fi

# --- Backend compilation -----------------------------------------------------
ui_info "Compiling backend binary..."

# Locate go even when PATH comes from a non-bash login environment (e.g. the
# user's go lives in their zsh .zshrc, invisible to `bash -l`).
if ! ui_have go; then
  go_bin="$(find_go_bin || true)"
  [[ -n "$go_bin" ]] && export PATH="$(dirname "$go_bin"):$PATH"
fi
ui_have go || ui_die "go not found on PATH or in common install locations (/usr/local/go/bin, ~/go/bin, snap)."

cd backend

LDFLAGS="-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE}"
go mod tidy

if go build -ldflags "$LDFLAGS" -o "./${BIN_NAME}" .; then
  ui_ok "Backend binary created at ${UI_BOLD}backend/${BIN_NAME}${UI_NC}"
else
  ui_die "Backend compilation failed!"
fi

# --- Summary -----------------------------------------------------------------
cd "$PRJ_ROOT"
ELAPSED=$((SECONDS - START_SECONDS))

if [[ $INLINE == 1 ]]; then
  ls -lh "backend/${BIN_NAME}"
  exit 0
fi

ui_header "Build complete" "finished in ${ELAPSED}s"
ls -lh "backend/${BIN_NAME}"
ui_detail "Next step — point your systemd unit at the served binary:"
ui_detail "WorkingDirectory=$(pwd)/backend"
ui_detail "ExecStart=$(pwd)/backend/${BIN_NAME}"
ui_rule 54
