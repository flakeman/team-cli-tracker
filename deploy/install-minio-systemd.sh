#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/minio.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  echo "copy deploy/minio.env.example -> deploy/minio.env and edit values"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/install-minio-systemd.sh [deploy/minio.env]"
  exit 1
fi

MINIO_BIND_ADDRESS="${MINIO_BIND_ADDRESS:-10.20.0.1}"
MINIO_API_PORT="${MINIO_API_PORT:-9000}"
MINIO_CONSOLE_PORT="${MINIO_CONSOLE_PORT:-9001}"
MINIO_DISTRIBUTED_ENDPOINTS="${MINIO_DISTRIBUTED_ENDPOINTS:-http://10.20.0.1:9000/data}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-minioadmin}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-minioadmin}"

cat >/etc/default/minio <<EOF
MINIO_BIND_ADDRESS=${MINIO_BIND_ADDRESS}
MINIO_API_PORT=${MINIO_API_PORT}
MINIO_CONSOLE_PORT=${MINIO_CONSOLE_PORT}
MINIO_DISTRIBUTED_ENDPOINTS=${MINIO_DISTRIBUTED_ENDPOINTS}
MINIO_ROOT_USER=${MINIO_ROOT_USER}
MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
EOF

cat >/etc/systemd/system/minio.service <<'EOF'
[Unit]
Description=MinIO
After=network-online.target
Wants=network-online.target

[Service]
User=minio
Group=minio
EnvironmentFile=/etc/default/minio
ExecStart=/bin/bash -lc '/usr/local/bin/minio server --address "${MINIO_BIND_ADDRESS}:${MINIO_API_PORT}" --console-address "${MINIO_BIND_ADDRESS}:${MINIO_CONSOLE_PORT}" ${MINIO_DISTRIBUTED_ENDPOINTS}'
WorkingDirectory=/var/lib/minio
Restart=always
RestartSec=2
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable minio
systemctl restart minio
systemctl --no-pager --full status minio || true

echo "minio service installed"
