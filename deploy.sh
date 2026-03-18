#!/bin/bash
set -e

# Configurable via env vars, with sensible defaults
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
IMAGE_NAME="${IMAGE_NAME:-ssh-portfolio}"
CONTAINER_NAME="${CONTAINER_NAME:-ssh-portfolio}"
SSH_KEY_DIR="${SSH_KEY_DIR:-$APP_DIR/.ssh}"

# Auto-detect port from Dockerfile EXPOSE — no hardcoding needed
PORT="${PORT:-$(grep -m1 '^EXPOSE' "$APP_DIR/Dockerfile" | awk '{print $2}')}"

if [ -z "$PORT" ]; then
  echo "ERROR: Could not detect port from Dockerfile. Set PORT env var manually."
  exit 1
fi

echo "==> Deploying $CONTAINER_NAME (port $PORT)"

# 1. Install Docker if missing
if ! command -v docker &>/dev/null; then
  echo "==> Docker not found, installing..."
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker "$USER"
  echo "NOTE: Run 'newgrp docker' for group change to take effect, then re-run this script."
  exit 0
fi

# 2. Build image
echo "==> Building image..."
cd "$APP_DIR"
docker build -t "$IMAGE_NAME" .

# 3. Stop and remove old container if running
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
  echo "==> Removing old container..."
  docker stop "$CONTAINER_NAME" || true
  docker rm "$CONTAINER_NAME" || true
fi

# 4. Run new container
echo "==> Starting container..."
docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  -p "${PORT}:${PORT}" \
  -v "${SSH_KEY_DIR}:/app/.ssh" \
  "$IMAGE_NAME"

# 5. Open firewall port via UFW (if available)
if command -v ufw &>/dev/null; then
  echo "==> Configuring UFW for port $PORT..."
  sudo ufw allow "${PORT}/tcp"
  sudo ufw reload
fi

echo ""
echo "==> Done! Container status:"
docker ps --filter "name=$CONTAINER_NAME" --format "  {{.Names}}  {{.Status}}  {{.Ports}}"
echo ""
echo "==> Test: ssh -p $PORT <server-ip>"
