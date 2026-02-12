#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/cluster.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root"
  exit 1
fi

if [[ ! -f "${REPO_DIR}/configs/node-${NODE_ID}.yaml" ]]; then
  echo "missing node config: ${REPO_DIR}/configs/node-${NODE_ID}.yaml"
  echo "run gen-node-config.sh first"
  exit 1
fi

cat >/etc/systemd/system/team-cli-tracker.service <<EOF
[Unit]
Description=team-cli-tracker node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${APP_USER}
Group=${APP_GROUP}
WorkingDirectory=${REPO_DIR}
Environment=PATH=/usr/local/go/bin:/usr/bin:/bin
ExecStart=/usr/local/go/bin/go run ./cmd/node
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable team-cli-tracker.service
systemctl restart team-cli-tracker.service
systemctl --no-pager --full status team-cli-tracker.service || true

echo "service installed: team-cli-tracker.service"

