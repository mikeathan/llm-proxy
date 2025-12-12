#!/usr/bin/env bash
set -euo pipefail

# Go to script directory (project root)
cd "$(dirname "$0")"

echo "Building llm-proxy..."

go mod tidy

# Build binary
go build -o ../llm-proxy .

echo "Build complete."
echo "Binary: $(cd .. && pwd)/llm-proxy"
ls -lh ../llm-proxy