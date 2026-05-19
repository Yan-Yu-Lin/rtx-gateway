#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIST_DIR="$ROOT_DIR/dist/deploy"
REMOTE_HOST="${REMOTE_HOST:-jason-ts}"
REMOTE_APP_USER="${REMOTE_APP_USER:-jason}"
REMOTE_SERVICE_USER="${REMOTE_SERVICE_USER:-rtx-gateway}"
REMOTE_SERVICE_GROUP="${REMOTE_SERVICE_GROUP:-rtx-gateway}"

GATEWAY_BIN="$DIST_DIR/rtx-gateway-linux-amd64"
DASHBOARD_TAR="$DIST_DIR/rtx-gateway-dashboard-output.tar.gz"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need go
need npm
need ssh
need scp
need tar

mkdir -p "$DIST_DIR"

echo "==> Building Go gateway for linux/amd64"
(
  cd "$ROOT_DIR"
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$GATEWAY_BIN" ./cmd/rtx-gateway
)

echo "==> Building Nuxt dashboard"
(
  cd "$ROOT_DIR/dashboard"
  npm ci
  npm run build
)

echo "==> Packing dashboard .output"
tar -C "$ROOT_DIR/dashboard/.output" -czf "$DASHBOARD_TAR" .

echo "==> Uploading artifacts to $REMOTE_HOST"
scp "$GATEWAY_BIN" "$REMOTE_HOST:/tmp/rtx-gateway"
scp "$DASHBOARD_TAR" "$REMOTE_HOST:/tmp/rtx-gateway-dashboard-output.tar.gz"
scp "$ROOT_DIR/deploy/systemd/rtx-gateway.service" "$REMOTE_HOST:/tmp/rtx-gateway.service"
scp "$ROOT_DIR/deploy/rtx-gateway.env.example" "$REMOTE_HOST:/tmp/rtx-gateway.env.example"
scp "$ROOT_DIR/deploy/dashboard.env.example" "$REMOTE_HOST:/tmp/rtx-gateway-dashboard.env.example"
for site in rtx-llm.arthurlin.dev rtx-ocr.arthurlin.dev gateway.arthurlin.dev; do
  scp "$ROOT_DIR/deploy/nginx/$site" "$REMOTE_HOST:/tmp/$site"
done
for conf in rtx-gateway-security.conf; do
  scp "$ROOT_DIR/deploy/nginx/conf.d/$conf" "$REMOTE_HOST:/tmp/$conf"
done

echo "==> Installing files on $REMOTE_HOST"
ssh "$REMOTE_HOST" \
  "REMOTE_APP_USER='$REMOTE_APP_USER' REMOTE_SERVICE_USER='$REMOTE_SERVICE_USER' REMOTE_SERVICE_GROUP='$REMOTE_SERVICE_GROUP' bash -s" <<'REMOTE_SCRIPT'
set -Eeuo pipefail

sudo install -d -o root -g root -m 0755 /etc/rtx-gateway
sudo install -d -o root -g root -m 0755 /opt/rtx-gateway
sudo install -d -o "$REMOTE_SERVICE_USER" -g "$REMOTE_SERVICE_GROUP" -m 0750 /var/lib/rtx-gateway /var/log/rtx-gateway

sudo install -o root -g root -m 0755 /tmp/rtx-gateway /opt/rtx-gateway/rtx-gateway
sudo install -o root -g root -m 0644 /tmp/rtx-gateway.service /etc/systemd/system/rtx-gateway.service

sudo install -o root -g "$REMOTE_SERVICE_GROUP" -m 0640 /tmp/rtx-gateway.env.example /etc/rtx-gateway/rtx-gateway.env.example
if [ ! -f /etc/rtx-gateway/rtx-gateway.env ]; then
  sudo install -o root -g "$REMOTE_SERVICE_GROUP" -m 0640 /tmp/rtx-gateway.env.example /etc/rtx-gateway/rtx-gateway.env
  echo "created /etc/rtx-gateway/rtx-gateway.env from template; edit secrets before production use" >&2
fi

sudo install -o root -g "$REMOTE_APP_USER" -m 0640 /tmp/rtx-gateway-dashboard.env.example /etc/rtx-gateway/dashboard.env.example
if [ ! -f /etc/rtx-gateway/dashboard.env ]; then
  sudo install -o root -g "$REMOTE_APP_USER" -m 0640 /tmp/rtx-gateway-dashboard.env.example /etc/rtx-gateway/dashboard.env
  echo "created /etc/rtx-gateway/dashboard.env from template; edit secrets before production use" >&2
fi

sudo install -d -o "$REMOTE_APP_USER" -g "$REMOTE_APP_USER" -m 0755 /opt/rtx-gateway/dashboard
if [ -d /opt/rtx-gateway/dashboard/.output ]; then
  sudo mv /opt/rtx-gateway/dashboard/.output "/opt/rtx-gateway/dashboard/.output.$(date +%Y%m%d%H%M%S).bak"
fi
sudo install -d -o "$REMOTE_APP_USER" -g "$REMOTE_APP_USER" -m 0755 /opt/rtx-gateway/dashboard/.output
sudo tar -xzf /tmp/rtx-gateway-dashboard-output.tar.gz -C /opt/rtx-gateway/dashboard/.output
sudo chown -R "$REMOTE_APP_USER:$REMOTE_APP_USER" /opt/rtx-gateway/dashboard

for site in rtx-llm.arthurlin.dev rtx-ocr.arthurlin.dev gateway.arthurlin.dev; do
  sudo install -o root -g root -m 0644 "/tmp/$site" "/etc/nginx/sites-available/$site"
  sudo ln -sfn "/etc/nginx/sites-available/$site" "/etc/nginx/sites-enabled/$site"
done
for conf in rtx-gateway-security.conf; do
  sudo install -o root -g root -m 0644 "/tmp/$conf" "/etc/nginx/conf.d/$conf"
done

sudo systemctl daemon-reload
sudo systemctl enable rtx-gateway
sudo systemctl restart rtx-gateway

sudo nginx -t
sudo systemctl reload nginx

sudo -iu "$REMOTE_APP_USER" bash -s <<'APP_SCRIPT'
set -Eeuo pipefail
export FNM_DIR="$HOME/.local/share/fnm"
export PATH="$FNM_DIR:$PATH"
if [ -x "$FNM_DIR/fnm" ]; then
  eval "$("$FNM_DIR/fnm" env --shell bash)"
  "$FNM_DIR/fnm" use 20 >/dev/null || true
fi

set -a
. /etc/rtx-gateway/dashboard.env
set +a

if pm2 describe rtx-gateway-dashboard >/dev/null 2>&1; then
  pm2 restart rtx-gateway-dashboard --update-env
else
  pm2 start /opt/rtx-gateway/dashboard/.output/server/index.mjs --name rtx-gateway-dashboard --time
fi
pm2 save
APP_SCRIPT
REMOTE_SCRIPT

echo "==> Deployment files installed"
echo "Next checks:"
echo "  ssh $REMOTE_HOST 'sudo systemctl status rtx-gateway --no-pager'"
echo "  ssh $REMOTE_HOST 'pm2 status rtx-gateway-dashboard'"
echo "  ssh $REMOTE_HOST 'sudo nginx -t'"
