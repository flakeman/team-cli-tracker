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

### 7) Запуск ноды
```bash
go run ./cmd/node
```

(временная команда для MVP-каркаса; systemd unit будет расширяться дальше)

### 8) Systemd шаблон (когда будут аргументы запуска ноды)
Пример пути юнита:
- `/etc/systemd/system/team-cli-tracker.service`

Далее:
```bash
sudo systemctl daemon-reload
sudo systemctl enable team-cli-tracker
sudo systemctl start team-cli-tracker
sudo systemctl status team-cli-tracker
```

### 9) Ops-чеклист
- Только SSH key auth.
- Отключить парольный вход в `sshd_config`.
- Держать включенным fail2ban.
- Поддерживать 3-node quorum для policy-операций.

### 10) Следующий шаг деплоя
После настройки сети нод реализовать и выкатить:
1. Генерацию ключей ноды.
2. Репликацию подписанных событий.
3. Health и sync status endpoints.
4. Service unit + rolling restart script.

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

### 7) Run Node
```bash
go run ./cmd/node
```

(temporary command for MVP skeleton; service unit will be added in next iteration)

### 8) Systemd Template (when node args are introduced)
Example unit path:
- `/etc/systemd/system/team-cli-tracker.service`

Then:
```bash
sudo systemctl daemon-reload
sudo systemctl enable team-cli-tracker
sudo systemctl start team-cli-tracker
sudo systemctl status team-cli-tracker
```

### 9) Ops Checklist
- SSH key auth only.
- Disable password login in `sshd_config`.
- Keep fail2ban enabled.
- Keep 3-node quorum alive for policy actions.

### 10) Next Deployment Step
After node networking is ready, implement and deploy:
1. Node key generation.
2. Signed event replication.
3. Health and sync status endpoints.
4. Service unit + rolling restart script.
