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
  echo "run as root: sudo bash deploy/install-minio.sh [deploy/minio.env]"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y curl ca-certificates

if ! id minio >/dev/null 2>&1; then
  useradd --system --create-home --shell /usr/sbin/nologin minio
fi

mkdir -p /opt/minio /var/lib/minio/data /var/log/minio /etc/minio
chown -R minio:minio /opt/minio /var/lib/minio /var/log/minio /etc/minio

if [[ ! -x /usr/local/bin/minio ]]; then
  curl -fsSL https://dl.min.io/server/minio/release/linux-amd64/minio -o /usr/local/bin/minio
  chmod +x /usr/local/bin/minio
fi

if [[ ! -x /usr/local/bin/mc ]]; then
  curl -fsSL https://dl.min.io/client/mc/release/linux-amd64/mc -o /usr/local/bin/mc
  chmod +x /usr/local/bin/mc
fi

echo "minio binaries installed"
