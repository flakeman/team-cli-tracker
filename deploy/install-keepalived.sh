#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/keepalived.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  echo "copy deploy/keepalived.env.example -> deploy/keepalived.env and edit values"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/install-keepalived.sh [deploy/keepalived.env]"
  exit 1
fi

KA_INTERFACE="${KA_INTERFACE:-wg0}"
KA_VRID="${KA_VRID:-51}"
KA_PRIORITY="${KA_PRIORITY:-120}"
KA_AUTH_PASS="${KA_AUTH_PASS:-boardinternal}"
KA_VIP="${KA_VIP:-10.20.0.10/24}"
KA_STATE="${KA_STATE:-BACKUP}"
KA_CHECK_HOST="${KA_CHECK_HOST:-127.0.0.1}"
KA_CHECK_PORT="${KA_CHECK_PORT:-8080}"
KA_CHECK_PATH="${KA_CHECK_PATH:-/api/v1/healthz}"
KA_UNICAST_SRC_IP="${KA_UNICAST_SRC_IP:-}"
KA_UNICAST_PEERS="${KA_UNICAST_PEERS:-}"

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y keepalived curl

mkdir -p /etc/keepalived

cat >/etc/keepalived/check_board_gateway.sh <<EOF
#!/usr/bin/env bash
set -e
curl -fsS --max-time 2 "http://${KA_CHECK_HOST}:${KA_CHECK_PORT}${KA_CHECK_PATH}" >/dev/null
EOF
chmod +x /etc/keepalived/check_board_gateway.sh

unicast_block=""
if [[ -n "${KA_UNICAST_SRC_IP}" ]] && [[ -n "${KA_UNICAST_PEERS}" ]]; then
  IFS=',' read -r -a peers <<< "${KA_UNICAST_PEERS}"
  peer_lines=""
  for p in "${peers[@]}"; do
    x="$(echo "$p" | xargs)"
    [[ -z "$x" ]] && continue
    peer_lines+=$'    '"${x}"$'\n'
  done
  if [[ -n "${peer_lines}" ]]; then
    unicast_block="  unicast_src_ip ${KA_UNICAST_SRC_IP}
  unicast_peer {
${peer_lines%\\n}
  }"
  fi
fi

cat >/etc/keepalived/keepalived.conf <<EOF
global_defs {
  enable_script_security
  script_user root
}

vrrp_script chk_gateway {
  script "/etc/keepalived/check_board_gateway.sh"
  interval 2
  timeout 2
  rise 2
  fall 2
}

vrrp_instance VI_${KA_VRID} {
  state ${KA_STATE}
  interface ${KA_INTERFACE}
  virtual_router_id ${KA_VRID}
  priority ${KA_PRIORITY}
  advert_int 1

${unicast_block}

  authentication {
    auth_type PASS
    auth_pass ${KA_AUTH_PASS}
  }

  track_script {
    chk_gateway
  }

  virtual_ipaddress {
    ${KA_VIP}
  }
}
EOF

systemctl daemon-reload
systemctl enable keepalived
systemctl restart keepalived
systemctl --no-pager --full status keepalived || true

echo "keepalived installed"
echo "state=${KA_STATE} interface=${KA_INTERFACE} vip=${KA_VIP}"
