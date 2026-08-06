#!/bin/bash
set -e

# Configurable via env vars, with sensible defaults
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
IMAGE_NAME="${IMAGE_NAME:-ssh-portfolio}"
CONTAINER_NAME="${CONTAINER_NAME:-ssh-portfolio}"
SSH_KEY_DIR="${SSH_KEY_DIR:-$APP_DIR/.ssh}"
DATA_DIR="${DATA_DIR:-$APP_DIR/data}"
DOMAIN="${DOMAIN:-ssh.mornqing.com}"
LETSENCRYPT_EMAIL="${LETSENCRYPT_EMAIL:-}"
WEB_ROOT="${WEB_ROOT:-/var/www/html}"

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
mkdir -p "$DATA_DIR"
docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  -p "${PORT}:${PORT}" \
  -v "${SSH_KEY_DIR}:/app/.ssh" \
  -v "${DATA_DIR}:/app/data" \
  "$IMAGE_NAME"

# 5. Open firewall port via UFW (if available)
if command -v ufw &>/dev/null; then
  echo "==> Configuring UFW for port $PORT, 80 and 443..."
  sudo ufw allow "${PORT}/tcp"
  sudo ufw allow 80/tcp
  sudo ufw allow 443/tcp
  sudo ufw reload
fi

# 6. Deploy web landing page via Nginx and enable HTTPS
echo "==> Deploying web landing page..."
if ! command -v nginx &>/dev/null; then
  echo "==> Nginx not found, installing..."
  sudo apt-get update -qq && sudo apt-get install -y nginx
fi
if ! command -v certbot &>/dev/null; then
  echo "==> Certbot not found, installing..."
  sudo apt-get update -qq && sudo apt-get install -y certbot
fi

sudo mkdir -p "$WEB_ROOT/.well-known/acme-challenge"
sudo cp "$APP_DIR/web/index.html" "$WEB_ROOT/index.html"

# Start with HTTP so Let's Encrypt can validate the domain on first deploy.
sudo tee /etc/nginx/sites-available/ssh-portfolio > /dev/null <<NGINX
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name $DOMAIN;
    root $WEB_ROOT;
    index index.html;

    location / {
        try_files \$uri \$uri/ =404;
    }
}
NGINX

sudo ln -sf /etc/nginx/sites-available/ssh-portfolio /etc/nginx/sites-enabled/ssh-portfolio
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx

if [ ! -f "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]; then
  echo "==> Requesting Let's Encrypt certificate for $DOMAIN..."
  CERTBOT_ARGS=(
    certonly
    --webroot
    --webroot-path "$WEB_ROOT"
    --domain "$DOMAIN"
    --non-interactive
    --agree-tos
  )
  if [ -n "$LETSENCRYPT_EMAIL" ]; then
    CERTBOT_ARGS+=(--email "$LETSENCRYPT_EMAIL")
  else
    CERTBOT_ARGS+=(--register-unsafely-without-email)
  fi
  sudo certbot "${CERTBOT_ARGS[@]}"
fi

sudo tee /etc/nginx/sites-available/ssh-portfolio > /dev/null <<NGINX
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name $DOMAIN;

    location ^~ /.well-known/acme-challenge/ {
        root $WEB_ROOT;
    }

    location / {
        return 301 https://$DOMAIN\$request_uri;
    }
}

server {
    listen 443 ssl http2 default_server;
    listen [::]:443 ssl http2 default_server;
    server_name $DOMAIN;

    ssl_certificate /etc/letsencrypt/live/$DOMAIN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$DOMAIN/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    root $WEB_ROOT;
    index index.html;

    location / {
        try_files \$uri \$uri/ =404;
    }
}
NGINX

sudo mkdir -p /etc/letsencrypt/renewal-hooks/deploy
sudo tee /etc/letsencrypt/renewal-hooks/deploy/reload-nginx > /dev/null <<'HOOK'
#!/bin/sh
systemctl reload nginx
HOOK
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx

if systemctl list-unit-files certbot.timer &>/dev/null; then
  sudo systemctl enable --now certbot.timer
fi

sudo nginx -t && sudo systemctl reload nginx

echo ""
echo "==> Done! Container status:"
docker ps --filter "name=$CONTAINER_NAME" --format "  {{.Names}}  {{.Status}}  {{.Ports}}"
echo ""
echo "==> SSH:  ssh -p $PORT <server-ip>"
echo "==> Web:  https://$DOMAIN"
