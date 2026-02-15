#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/wireguard.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  echo "copy deploy/wireguard.env.example -> deploy/wireguard.env and fill values"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/install-wireguard.sh [deploy/wireguard.env]"
  exit 1
fi

if [[ -z "${WG_PRIVATE_KEY:-}" ]] || [[ -z "${WG_ADDRESS_CIDR:-}" ]]; then
  echo "WG_PRIVATE_KEY and WG_ADDRESS_CIDR are required"
  exit 1
fi

iface="${WG_INTERFACE:-wg0}"
port="${WG_PORT:-51820}"

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y wireguard

mkdir -p /etc/wireguard
chmod 700 /etc/wireguard

cat >/etc/wireguard/${iface}.conf <<EOF
[Interface]
Address = ${WG_ADDRESS_CIDR}
ListenPort = ${port}
PrivateKey = ${WG_PRIVATE_KEY}
EOF

if [[ -n "${WG_PEERS:-}" ]]; then
  IFS=',' read -r -a peers <<<"${WG_PEERS}"
  for p in "${peers[@]}"; do
    row="$(echo "$p" | xargs)"
    [[ -z "$row" ]] && continue
    IFS='|' read -r peer_name peer_pub peer_endpoint peer_allowed <<<"$row"
    if [[ -z "${peer_pub:-}" ]] || [[ -z "${peer_allowed:-}" ]]; then
      echo "skip malformed peer: $row"
      continue
    fi
    {
      echo ""
      echo "[Peer]"
      echo "# ${peer_name:-peer}"
      echo "PublicKey = ${peer_pub}"
      echo "AllowedIPs = ${peer_allowed}"
      if [[ -n "${peer_endpoint:-}" ]]; then
        echo "Endpoint = ${peer_endpoint}"
        echo "PersistentKeepalive = 25"
      fi
    } >>/etc/wireguard/${iface}.conf
  done
fi

chmod 600 /etc/wireguard/${iface}.conf
systemctl enable wg-quick@${iface}
systemctl restart wg-quick@${iface}
systemctl --no-pager --full status wg-quick@${iface} || true

echo "wireguard installed on ${iface}"
echo "node: ${WG_NODE_ID:-unknown}"
