#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${1:-$SCRIPT_DIR/cluster.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE"
  echo "copy deploy/cluster.env.example -> deploy/cluster.env and edit values"
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo bash deploy/bootstrap-debian13.sh [deploy/cluster.env]"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt update
apt -y upgrade
apt -y install git curl ca-certificates ufw fail2ban tar

timedatectl set-timezone UTC || true

if ! id "$APP_USER" >/dev/null 2>&1; then
  useradd --system --create-home --shell /bin/bash "$APP_USER"
fi

ufw --force default deny incoming
ufw --force default allow outgoing
ufw --force allow 22/tcp
ufw --force allow 4101/tcp
ufw --force allow 4101/udp
ufw --force enable

if [[ ! -x /usr/local/go/bin/go ]] || [[ "$(/usr/local/go/bin/go version 2>/dev/null || true)" != *"go${GO_VERSION}"* ]]; then
  curl -fsSLO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
  rm -f "go${GO_VERSION}.linux-amd64.tar.gz"
fi

if [[ ! -d "$REPO_DIR/.git" ]]; then
  git clone "$REPO_URL" "$REPO_DIR"
fi

cd "$REPO_DIR"
git fetch origin
git checkout "$REPO_BRANCH"
git pull --ff-only origin "$REPO_BRANCH"

chown -R "$APP_USER:$APP_GROUP" "$REPO_DIR"

cat >/etc/profile.d/team-cli-tracker-go.sh <<'EOF'
export PATH=$PATH:/usr/local/go/bin
EOF

echo "bootstrap complete"
echo "repo: $REPO_DIR"
echo "next:"
echo "  sudo -u $APP_USER bash $REPO_DIR/deploy/gen-node-config.sh $REPO_DIR/deploy/cluster.env"
echo "  sudo bash $REPO_DIR/deploy/install-systemd-service.sh $REPO_DIR/deploy/cluster.env"

