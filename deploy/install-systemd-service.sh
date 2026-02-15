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
LISTEN_ADDR="${LISTEN_ADDR:-:4101}"
NODE_PUBLIC_URL="${NODE_PUBLIC_URL:-http://${NODE_ID}:4101}"
DATA_DIR="${DATA_DIR:-${REPO_DIR}/data}"
SECURE_MODE_REQUIRED="${SECURE_MODE_REQUIRED:-true}"
ATTACHMENT_BACKEND="${ATTACHMENT_BACKEND:-local}"
ATTACHMENT_S3_ENDPOINT="${ATTACHMENT_S3_ENDPOINT:-}"
ATTACHMENT_S3_BUCKET="${ATTACHMENT_S3_BUCKET:-}"
ATTACHMENT_S3_ACCESS_KEY="${ATTACHMENT_S3_ACCESS_KEY:-}"
ATTACHMENT_S3_SECRET_KEY="${ATTACHMENT_S3_SECRET_KEY:-}"
ATTACHMENT_S3_REGION="${ATTACHMENT_S3_REGION:-us-east-1}"
ATTACHMENT_S3_SECURE="${ATTACHMENT_S3_SECURE:-false}"

# Fallback: derive peer URLs from NODE_PEERS IDs if explicit URL list is not provided.
if [[ -z "${NODE_PEERS_URLS}" ]] && [[ -n "${NODE_PEERS:-}" ]]; then
  derived=""
  IFS=',' read -r -a ids <<<"${NODE_PEERS}"
  for id in "${ids[@]}"; do
    p="$(echo "$id" | xargs)"
    [[ -z "$p" ]] && continue
    if [[ -n "$derived" ]]; then
      derived="${derived},"
    fi
    derived="${derived}http://${p}:4101"
  done
  NODE_PEERS_URLS="${derived}"
fi

mkdir -p "${DATA_DIR}"

cat >/etc/default/team-cli-tracker <<EOF
PROJECT_ID=${PROJECT_ID}
NODE_ID=${NODE_ID}
NODE_PEERS=${NODE_PEERS}
NODE_PEERS_URLS=${NODE_PEERS_URLS}
LISTEN_ADDR=${LISTEN_ADDR}
NODE_PUBLIC_URL=${NODE_PUBLIC_URL}
DATA_DIR=${DATA_DIR}
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
ExecStart=/bin/bash -lc '/usr/local/go/bin/go run ./cmd/node serve --project-id "${PROJECT_ID}" --listen "${LISTEN_ADDR}" --node-id "${NODE_ID}" --public-url "${NODE_PUBLIC_URL}" --peers "${NODE_PEERS_URLS}" --data-dir "${DATA_DIR}" --secure-mode-required="${SECURE_MODE_REQUIRED}" --attachment-backend="${ATTACHMENT_BACKEND}" --attachment-s3-endpoint="${ATTACHMENT_S3_ENDPOINT}" --attachment-s3-bucket="${ATTACHMENT_S3_BUCKET}" --attachment-s3-access-key="${ATTACHMENT_S3_ACCESS_KEY}" --attachment-s3-secret-key="${ATTACHMENT_S3_SECRET_KEY}" --attachment-s3-region="${ATTACHMENT_S3_REGION}" --attachment-s3-secure="${ATTACHMENT_S3_SECURE}"'
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
