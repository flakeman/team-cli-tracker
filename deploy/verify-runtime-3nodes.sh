#!/usr/bin/env bash
set -euo pipefail

# Verifies tracker runtime on three nodes:
# 1) systemd service is active
# 2) secure-mode-required=true is present in systemd environment
# 3) local /healthz responds with {"status":"ok"}
#
# Run from a host that has SSH key access to all targets.
# Example:
#   bash deploy/verify-runtime-3nodes.sh vova srv1.example.internal srv2.example.internal srv3.example.internal

SSH_USER="${1:-vova}"
N1="${2:-srv1.example.internal}"
N2="${3:-srv2.example.internal}"
N3="${4:-srv3.example.internal}"
SERVICE_NAME="${SERVICE_NAME:-team-cli-tracker}"

check_node() {
  local host="$1"
  echo "==> ${host}"
  ssh -o BatchMode=yes -o ConnectTimeout=8 "${SSH_USER}@${host}" "\
set -euo pipefail; \
systemctl is-active ${SERVICE_NAME}.service >/dev/null; \
grep -q '^SECURE_MODE_REQUIRED=true$' /etc/default/${SERVICE_NAME}; \
curl -fsS http://127.0.0.1:4101/healthz | grep -q '\"status\":\"ok\"'; \
echo ok"
}

check_node "${N1}"
check_node "${N2}"
check_node "${N3}"
echo "runtime verification passed on all nodes"

