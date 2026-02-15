#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <archive.tar.gz> <target_data_dir>"
  exit 1
fi

ARCHIVE="$1"
TARGET="$2"
SHA_FILE="${ARCHIVE}.sha256"

if [[ ! -f "$ARCHIVE" ]]; then
  echo "archive not found: $ARCHIVE"
  exit 1
fi

if [[ -f "$SHA_FILE" ]]; then
  sha256sum -c "$SHA_FILE"
else
  echo "warning: checksum file not found: $SHA_FILE"
fi

mkdir -p "$TARGET"
tar -C "$TARGET" -xzf "$ARCHIVE"

echo "restore complete: $TARGET"
