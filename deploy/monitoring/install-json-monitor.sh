#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK_SCRIPT="${SCRIPT_DIR}/check-json-metrics.sh"

if [[ ! -f "${CHECK_SCRIPT}" ]]; then
  echo "script not found: ${CHECK_SCRIPT}"
  exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/monitoring/install-json-monitor.sh [targets_csv]"
  exit 1
fi

TARGETS_CSV="${1:-srv1.abuztech.ru:4101,srv2.abuztech.ru:4101,srv3.abuztech.ru:4101}"

install -d -m 755 /opt/team-cli-tracker/monitoring
install -m 755 "${CHECK_SCRIPT}" /opt/team-cli-tracker/monitoring/check-json-metrics.sh

cat >/etc/systemd/system/team-cli-json-monitor.service <<EOF
[Unit]
Description=team-cli JSON metrics monitor
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/opt/team-cli-tracker/monitoring/check-json-metrics.sh "${TARGETS_CSV}"
EOF

cat >/etc/systemd/system/team-cli-json-monitor.timer <<'EOF'
[Unit]
Description=team-cli JSON metrics monitor timer

[Timer]
OnBootSec=1min
OnUnitActiveSec=1min
Unit=team-cli-json-monitor.service

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now team-cli-json-monitor.timer
systemctl start team-cli-json-monitor.service || true
systemctl --no-pager --full status team-cli-json-monitor.timer || true

echo "installed team-cli-json-monitor timer"
