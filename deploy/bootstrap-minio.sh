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

MINIO_BIND_ADDRESS="${MINIO_BIND_ADDRESS:-10.20.0.1}"
MINIO_API_PORT="${MINIO_API_PORT:-9000}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-minioadmin}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-minioadmin}"
MINIO_BUCKET="${MINIO_BUCKET:-team-cli-attachments}"

alias_name="localminio"
endpoint="http://${MINIO_BIND_ADDRESS}:${MINIO_API_PORT}"

/usr/local/bin/mc alias set "${alias_name}" "${endpoint}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}"
/usr/local/bin/mc mb --ignore-existing "${alias_name}/${MINIO_BUCKET}"
/usr/local/bin/mc anonymous set private "${alias_name}/${MINIO_BUCKET}"

echo "minio bootstrap complete: ${endpoint}/${MINIO_BUCKET}"
