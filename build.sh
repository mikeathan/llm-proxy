#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "Building llm-proxy..."

VERSION="${VERSION:-dev}"
if command -v git >/dev/null 2>&1; then
  COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "none")"
else
  COMMIT="none"
fi
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

go mod tidy

go build -ldflags "-X main.Version=$VERSION -X main.Commit=$COMMIT -X main.BuildDate=$BUILD_DATE" -o llm-proxy .

echo "Build complete."
ls -lh llm-proxy
