#!/usr/bin/env bash
set -Eeuo pipefail

REMOTE_HOST="${REMOTE_HOST:-jason-ts}"
DB_PATH="${RTX_GATEWAY_DB_PATH:-/var/lib/rtx-gateway/rtx-gateway.db}"

cat <<EOF
rtx-gateway applies migrations on service startup.
This script restarts the service on $REMOTE_HOST and checks SQLite integrity.
EOF

ssh "$REMOTE_HOST" "DB_PATH='$DB_PATH' bash -s" <<'REMOTE_SCRIPT'
set -Eeuo pipefail

sudo systemctl restart rtx-gateway
sudo systemctl status rtx-gateway --no-pager --lines=20

if sudo test -f "$DB_PATH"; then
  sudo -u rtx-gateway sqlite3 "$DB_PATH" 'pragma integrity_check;'
else
  echo "database not found yet: $DB_PATH" >&2
fi
REMOTE_SCRIPT
