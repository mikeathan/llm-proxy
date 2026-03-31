#!/usr/bin/env bash
set -euo pipefail

# Get project root
PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PRJ_ROOT"

echo "Building frontend..."
(cd frontend && npm install && npm run build)

echo "Building backend..."
# VERSION logic from original build script
VERSION="${VERSION:-}"
if command -v git >/dev/null 2>&1; then
  COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
  if [[ -z "${VERSION}" ]]; then
    VERSION="$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")"
  fi
  echo "Git commit: ${COMMIT}"
else
  COMMIT="none"
  if [[ -z "${VERSION}" ]]; then
    VERSION="dev"
  fi
  echo "Git not available; using commit: ${COMMIT}"
fi
echo "Version: ${VERSION}"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Build backend
cd backend
go mod tidy
go build -ldflags "-X main.Version=$VERSION -X main.Commit=$COMMIT -X main.BuildDate=$BUILD_DATE" -o ../llm-proxy .

echo "Build complete."
cd ..
ls -lh llm-proxy
