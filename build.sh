#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "Building llm-proxy..."

go mod tidy

go build -o llm-proxy .

echo "Build complete."
ls -lh llm-proxy