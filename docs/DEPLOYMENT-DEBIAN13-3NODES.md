# Deployment: Debian 13, 3 Nodes

## RU

Этот гайд подготавливает тестовый кластер из 3 нод для `team-cli-tracker`.

### Топология
- `node-1`: app + control (кандидат в лидеры)
- `node-2`: app peer
- `node-3`: app peer

Рекомендуемый минимум на VPS:
- 2 vCPU
- 2 GB RAM
- 20+ GB disk
- Debian 13 x64

### 1) Базовая подготовка (на каждой ноде)
```bash
sudo apt update && sudo apt -y upgrade
sudo apt -y install git curl ca-certificates ufw fail2ban

# опционально, но рекомендуется
sudo timedatectl set-timezone UTC
```

### 2) Firewall
```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp
sudo ufw allow 4101/tcp
sudo ufw allow 4101/udp
sudo ufw enable
```

### 3) Установка Go
```bash
curl -LO https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 4) Клонирование репозитория
```bash
git clone https://github.com/flakeman/team-cli-tracker.git
cd team-cli-tracker
go test ./...
```

### 5) Приватная сеть
Используй один из вариантов:
- Tailscale (рекомендуется)
- ZeroTier

Перед запуском нод они должны видеть друг друга по приватным адресам.

### 6) Конфиг нод
Создай конфиги из `configs/node.example.yaml`:
- `configs/node-1.yaml`
- `configs/node-2.yaml`
- `configs/node-3.yaml`

Укажи уникальные значения:
- `node_id`
- `listen_addr`
- список peers

### 7) Каноничный запуск (актуально)
Используй готовые скрипты из `deploy/`:
```bash
cp deploy/cluster.node-1.env.example deploy/cluster.env   # для node-1 (аналогично node-2/node-3)
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
cd /opt/team-cli-tracker && go build -o node ./cmd/node
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
```

Проверка:
```bash
systemctl is-active team-cli-tracker
curl -s http://127.0.0.1:4101/healthz
```

### 8) Systemd runtime
- Основной unit: `/etc/systemd/system/team-cli-tracker.service`
- Рекомендуемый режим: secure transport/auth (см. `docs/RELEASE-CHECKLIST.md`)

### 9) Ops-чеклист
- Только SSH key auth.
- Отключить парольный вход в `sshd_config`.
- Держать включенным fail2ban.
- Поддерживать 3-node quorum для policy-операций.

### 10) Примечание по legacy шагам
- `go run ./cmd/node` из ранних версий считать legacy (только локальный dev smoke).
- Для кластера использовать `deploy/*` скрипты и `systemd`.

---

## EN

This guide prepares a 3-node test cluster for `team-cli-tracker`.

### Topology
- `node-1`: app + control (candidate leader)
- `node-2`: app peer
- `node-3`: app peer

Recommended baseline per VPS:
- 2 vCPU
- 2 GB RAM
- 20+ GB disk
- Debian 13 x64

### 1) Base Setup (run on each node)
```bash
sudo apt update && sudo apt -y upgrade
sudo apt -y install git curl ca-certificates ufw fail2ban

# optional but recommended
sudo timedatectl set-timezone UTC
```

### 2) Firewall
```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp
sudo ufw allow 4101/tcp
sudo ufw allow 4101/udp
sudo ufw enable
```

### 3) Install Go
```bash
curl -LO https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 4) Clone Repo
```bash
git clone https://github.com/flakeman/team-cli-tracker.git
cd team-cli-tracker
go test ./...
```

### 5) Private Network
Use one of:
- Tailscale (recommended)
- ZeroTier

All 3 nodes must see each other over private addresses before node start.

### 6) Node Config
Create per-node configs from `configs/node.example.yaml`:
- `configs/node-1.yaml`
- `configs/node-2.yaml`
- `configs/node-3.yaml`

Set unique:
- `node_id`
- `listen_addr`
- peers list

### 7) Canonical Runtime (current)
Use `deploy/` scripts:
```bash
cp deploy/cluster.node-1.env.example deploy/cluster.env   # for node-1 (node-2/node-3 accordingly)
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
cd /opt/team-cli-tracker && go build -o node ./cmd/node
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
```

Verification:
```bash
systemctl is-active team-cli-tracker
curl -s http://127.0.0.1:4101/healthz
```

### 8) Systemd Runtime
- Primary unit: `/etc/systemd/system/team-cli-tracker.service`
- Recommended mode: secure transport/auth (see `docs/RELEASE-CHECKLIST.md`)

### 9) Ops Checklist
- SSH key auth only.
- Disable password login in `sshd_config`.
- Keep fail2ban enabled.
- Keep 3-node quorum alive for policy actions.

### 10) Legacy Note
- Treat `go run ./cmd/node` as legacy (local dev smoke only).
- For clusters, use `deploy/*` scripts and `systemd`.
