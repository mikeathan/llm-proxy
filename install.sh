#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="llm-proxy.service"
SERVICE_PATH="/etc/systemd/system/$SERVICE_NAME"
PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "Installing llm-proxy.service..."

# Copy service file
sudo cp "$PROJECT_ROOT/docs/services/$SERVICE_NAME" "$SERVICE_PATH"

# Reload systemd
sudo systemctl daemon-reload

# Enable boot startup
sudo systemctl enable "$SERVICE_NAME"

# Restart service
sudo systemctl restart "$SERVICE_NAME"

# Show status
sudo systemctl status "$SERVICE_NAME" --no-pager