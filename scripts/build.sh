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

# --- Initialization ---
cd "$PRJ_ROOT"
echo -e "${CYAN}${BOLD}==================================================${NC}"
info "Starting build for ${BOLD}llm-proxy${NC}..."
echo -e "${CYAN}${BOLD}==================================================${NC}"

# --- Versioning (Using Git Tags) ---
info "Retrieving version from Git tags..."
# Get the most recent tag, or fallback to 'dev' if no tags exist
VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
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
echo -e "\n${GREEN}${BOLD}✨ Build process complete! ✨${NC}"
ls -lh "backend/${BIN_NAME}"

echo -e "\n${CYAN}${BOLD}Next Step: Update your Systemd Service${NC}"
echo -e "Update your service file to use the backend folder as the working directory:"
echo -e "${BOLD}WorkingDirectory=$(pwd)/backend${NC}"
echo -e "${BOLD}ExecStart=$(pwd)/backend/${BIN_NAME}${NC}"
echo -e "--------------------------------------------------\n"
