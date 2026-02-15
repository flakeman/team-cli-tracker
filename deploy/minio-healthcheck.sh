#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/minio.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

MINIO_BIND_ADDRESS="${MINIO_BIND_ADDRESS:-10.20.0.1}"
MINIO_API_PORT="${MINIO_API_PORT:-9000}"
MINIO_BUCKET="${MINIO_BUCKET:-team-cli-attachments}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-minioadmin}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-minioadmin}"

endpoint="http://${MINIO_BIND_ADDRESS}:${MINIO_API_PORT}"
curl -fsS "${endpoint}/minio/health/live" >/dev/null
curl -fsS "${endpoint}/minio/health/ready" >/dev/null

/usr/local/bin/mc alias set localminio "${endpoint}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" >/dev/null
/usr/local/bin/mc ls "localminio/${MINIO_BUCKET}" >/dev/null

echo "ok: minio healthy and bucket accessible (${MINIO_BUCKET})"
