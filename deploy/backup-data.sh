#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <data_dir> <backup_dir>"
  exit 1
fi

DATA_DIR="$1"
BACKUP_DIR="$2"
TS="$(date -u +%Y%m%dT%H%M%SZ)"

if [[ ! -d "$DATA_DIR" ]]; then
  echo "data dir not found: $DATA_DIR"
  exit 1
fi

mkdir -p "$BACKUP_DIR"
ARCHIVE="${BACKUP_DIR}/team-cli-data-${TS}.tar.gz"
SHA="${ARCHIVE}.sha256"

tar -C "$DATA_DIR" -czf "$ARCHIVE" .
sha256sum "$ARCHIVE" >"$SHA"

echo "backup created: $ARCHIVE"
echo "checksum file: $SHA"
