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
  echo "run as root: sudo bash deploy/install-gateway-caddy.sh [deploy/gateway.env]"
  exit 1
fi

if [[ -z "${BOARD_DOMAIN:-}" ]] || [[ -z "${GATEWAY_UPSTREAMS:-}" ]]; then
  echo "BOARD_DOMAIN and GATEWAY_UPSTREAMS are required"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl

if [[ ! -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg ]]; then
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor >/usr/share/keyrings/caddy-stable-archive-keyring.gpg
fi
if [[ ! -f /etc/apt/sources.list.d/caddy-stable.list ]]; then
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' >/etc/apt/sources.list.d/caddy-stable.list
fi

apt-get update -y
apt-get install -y caddy

IFS=',' read -r -a raw_upstreams <<< "${GATEWAY_UPSTREAMS}"
upstreams=""
for u in "${raw_upstreams[@]}"; do
  x="$(echo "$u" | xargs)"
  [[ -z "$x" ]] && continue
  upstreams="${upstreams}        reverse_proxy ${x}\n"
done

if [[ -z "$upstreams" ]]; then
  echo "no upstreams parsed from GATEWAY_UPSTREAMS"
  exit 1
fi

if [[ "${GATEWAY_ENABLE_TLS:-true}" == "true" ]]; then
  site="${BOARD_DOMAIN}"
else
  site=":$(echo "${GATEWAY_LISTEN_PORT:-8080}" | xargs)"
fi

cat >/etc/caddy/Caddyfile <<EOF
{
    admin off
}

${site} {
    encode zstd gzip
    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "no-referrer"
    }
${upstreams%\\n}
}
EOF

systemctl daemon-reload
systemctl enable caddy
systemctl restart caddy
systemctl --no-pager --full status caddy || true

echo "gateway installed with Caddy"
echo "domain: ${BOARD_DOMAIN}"
echo "mode: ${GATEWAY_MODE:-public_proxy}"
