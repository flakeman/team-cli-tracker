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

PROJECT_ID="${PROJECT_ID:-OPS}"
NODE_PEERS_URLS="${NODE_PEERS_URLS:-}"
SECURE_MODE_REQUIRED="${SECURE_MODE_REQUIRED:-true}"
ATTACHMENT_BACKEND="${ATTACHMENT_BACKEND:-local}"
ATTACHMENT_S3_ENDPOINT="${ATTACHMENT_S3_ENDPOINT:-}"
ATTACHMENT_S3_BUCKET="${ATTACHMENT_S3_BUCKET:-}"
ATTACHMENT_S3_ACCESS_KEY="${ATTACHMENT_S3_ACCESS_KEY:-}"
ATTACHMENT_S3_SECRET_KEY="${ATTACHMENT_S3_SECRET_KEY:-}"
ATTACHMENT_S3_REGION="${ATTACHMENT_S3_REGION:-us-east-1}"
ATTACHMENT_S3_SECURE="${ATTACHMENT_S3_SECURE:-false}"

cat >/etc/default/team-cli-tracker <<EOF
PROJECT_ID=${PROJECT_ID}
NODE_ID=${NODE_ID}
NODE_PEERS=${NODE_PEERS}
NODE_PEERS_URLS=${NODE_PEERS_URLS}
SECURE_MODE_REQUIRED=${SECURE_MODE_REQUIRED}
ATTACHMENT_BACKEND=${ATTACHMENT_BACKEND}
ATTACHMENT_S3_ENDPOINT=${ATTACHMENT_S3_ENDPOINT}
ATTACHMENT_S3_BUCKET=${ATTACHMENT_S3_BUCKET}
ATTACHMENT_S3_ACCESS_KEY=${ATTACHMENT_S3_ACCESS_KEY}
ATTACHMENT_S3_SECRET_KEY=${ATTACHMENT_S3_SECRET_KEY}
ATTACHMENT_S3_REGION=${ATTACHMENT_S3_REGION}
ATTACHMENT_S3_SECURE=${ATTACHMENT_S3_SECURE}
EOF

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
EnvironmentFile=/etc/default/team-cli-tracker
ExecStart=/bin/bash -lc '/usr/local/go/bin/go run ./cmd/node serve --project-id "${PROJECT_ID}" --listen :4101 --node-id "${NODE_ID}" --public-url "http://${NODE_ID}.abuztech.ru:4101" --peers "${NODE_PEERS_URLS}" --data-dir "${PWD}/data" --secure-mode-required="${SECURE_MODE_REQUIRED}" --attachment-backend="${ATTACHMENT_BACKEND}" --attachment-s3-endpoint="${ATTACHMENT_S3_ENDPOINT}" --attachment-s3-bucket="${ATTACHMENT_S3_BUCKET}" --attachment-s3-access-key="${ATTACHMENT_S3_ACCESS_KEY}" --attachment-s3-secret-key="${ATTACHMENT_S3_SECRET_KEY}" --attachment-s3-region="${ATTACHMENT_S3_REGION}" --attachment-s3-secure="${ATTACHMENT_S3_SECURE}"'
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
