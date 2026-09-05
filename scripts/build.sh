#!/usr/bin/env bash
set -euo pipefail

# --- Configuration & UI Helpers ---
PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_NAME="llm-proxy"

# Colors for modern terminal output
BOLD='\033[1m'
CYAN='\033[0;36m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${BLUE}${BOLD}info${NC} $1"; }
success() { echo -e "${GREEN}${BOLD}success${NC} $1"; }
error() { echo -e "${RED}${BOLD}error${NC} $1"; exit 1; }

# Inline mode: callers that have their own UI (setup.sh, launch.sh) set
# BUILD_INLINE=1 to get lean output — no banner, no success sparkles, no
# systemd-advice footer (the caller says what happens next). Standalone runs
# keep the full decorated output.
INLINE=${BUILD_INLINE:-0}

# --- Initialization ---
cd "$PRJ_ROOT"
if [[ $INLINE != 1 ]]; then
  echo -e "${CYAN}${BOLD}==================================================${NC}"
  info "Starting build for ${BOLD}llm-proxy${NC}..."
  echo -e "${CYAN}${BOLD}==================================================${NC}"
fi

# --- Repo ownership guard ---
# Root-owned files in the tree (typically left behind by an earlier root-run
# build) make vite/npm fail mid-build with EACCES. Fail fast with the fix,
# or self-heal when the script itself runs as root.
if find . -user root -print -quit 2>/dev/null | grep -q .; then
  if [[ $EUID -eq 0 && -n "${SUDO_USER:-}" ]]; then
    info "Repairing root-owned files (from an earlier root-run build)..."
    chown -R "${SUDO_USER}":"$(id -gn "$SUDO_USER")" "$PRJ_ROOT"
    success "repo ownership repaired"
  else
    echo -e "${RED}${BOLD}error${NC} Found root-owned files in the repo (vite will fail with EACCES)."
    echo -e "Fix:  ${BOLD}sudo chown -R \"$(id -un)\":\"$(id -gn)\" $PRJ_ROOT${NC}"
    exit 1
  fi
fi

# --- Versioning (Using Git Tags) ---
info "Retrieving version from Git tags..."
# Get the most recent tag by version number, or fallback to 'dev' if no tags exist
VERSION=$(git tag --sort=-v:refname | head -n 1)
if [[ -z "${VERSION}" ]]; then
    VERSION="dev"
fi
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

info "Version: ${GREEN}${VERSION}${NC} (Commit: ${CYAN}${COMMIT}${NC})"

# --- Frontend Build ---
info "Building frontend assets..."
if (cd frontend && npm install && npm run build); then
    success "Frontend generated successfully."
else
    error "Frontend build failed!"
fi

# --- Backend Compilation ---
info "Compiling backend binary..."

# Locate go even when PATH comes from a non-bash login environment (e.g. the
# user's go lives in their zsh .zshrc, invisible to `bash -l`).
if ! command -v go >/dev/null 2>&1; then
  for d in /usr/local/go/bin "$HOME/go/bin" "$HOME/.local/bin" /snap/bin /usr/lib/go/bin /opt/go/bin; do
    if [[ -x "$d/go" ]]; then export PATH="$d:$PATH"; break; fi
  done
fi
if ! command -v go >/dev/null 2>&1; then
  error "go not found on PATH or in common install locations (/usr/local/go/bin, ~/go/bin, snap)."
fi

cd backend

# Go Linker flags
LDFLAGS="-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE}"

go mod tidy
# We build the binary directly inside the backend folder
if go build -ldflags "$LDFLAGS" -o "./${BIN_NAME}" .; then
    success "Backend binary created at ${BOLD}backend/${BIN_NAME}${NC}"
else
    error "Backend compilation failed!"
fi

# --- Success Summary ---
cd "$PRJ_ROOT"
if [[ $INLINE == 1 ]]; then
  ls -lh "backend/${BIN_NAME}"
  exit 0
fi

echo -e "\n${GREEN}${BOLD}✨ Build process complete! ✨${NC}"
ls -lh "backend/${BIN_NAME}"

echo -e "\n${CYAN}${BOLD}Next Step: Update your Systemd Service${NC}"
echo -e "Update your service file to use the backend folder as the working directory:"
echo -e "${BOLD}WorkingDirectory=$(pwd)/backend${NC}"
echo -e "${BOLD}ExecStart=$(pwd)/backend/${BIN_NAME}${NC}"
echo -e "--------------------------------------------------\n"
