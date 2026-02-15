#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/gateway.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  echo "copy deploy/gateway.env.example -> deploy/gateway.env and edit values"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/install-gateway-cloudflared.sh [deploy/gateway.env]"
  exit 1
fi

if [[ -z "${BOARD_DOMAIN:-}" ]] || [[ -z "${CLOUDFLARE_TUNNEL_NAME:-}" ]]; then
  echo "BOARD_DOMAIN and CLOUDFLARE_TUNNEL_NAME are required"
  exit 1
fi

if ! command -v cloudflared >/dev/null 2>&1; then
  arch="amd64"
  deb="/tmp/cloudflared.deb"
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}.deb" -o "$deb"
  dpkg -i "$deb" || apt-get -f install -y
fi

mkdir -p /etc/cloudflared

cat >/etc/cloudflared/config.yml <<EOF
tunnel: ${CLOUDFLARE_TUNNEL_NAME}
credentials-file: /etc/cloudflared/${CLOUDFLARE_TUNNEL_NAME}.json

ingress:
  - hostname: ${BOARD_DOMAIN}
    service: http://127.0.0.1:${GATEWAY_LISTEN_PORT:-8080}
  - service: http_status:404
EOF

cat <<EOF
cloudflared installed and config written: /etc/cloudflared/config.yml

Next manual steps (once per account):
1. cloudflared tunnel login
2. cloudflared tunnel create ${CLOUDFLARE_TUNNEL_NAME}
3. copy generated credentials json to:
   /etc/cloudflared/${CLOUDFLARE_TUNNEL_NAME}.json
4. cloudflared tunnel route dns ${CLOUDFLARE_TUNNEL_NAME} ${BOARD_DOMAIN}
5. systemctl enable --now cloudflared
EOF
