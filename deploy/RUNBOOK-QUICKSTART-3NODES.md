# Quickstart Runbook: 3 Nodes (Debian 13)

## RU

Этот runbook содержит точную последовательность команд для `node-1`, `node-2`, `node-3`.

### 0) Что нужно заранее
- 3 VPS на Debian 13 с доступом по SSH.
- Ноды должны видеть друг друга по приватной сети (рекомендуется Tailscale/ZeroTier).
- Репозиторий: `https://github.com/flakeman/team-cli-tracker`

### 1) На каждой ноде: clone
```bash
git clone https://github.com/flakeman/team-cli-tracker.git
cd team-cli-tracker
```

### 2) Конфиг по нодам

#### node-1
```bash
cp deploy/cluster.node-1.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Сгенерировать конфиг:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

#### node-2
```bash
cp deploy/cluster.node-2.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Сгенерировать конфиг:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

#### node-3
```bash
cp deploy/cluster.node-3.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Сгенерировать конфиг:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

### 3) Установка и запуск systemd сервиса
Запусти на каждой ноде:
```bash
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
```

### 4) Проверка
На каждой ноде:
```bash
systemctl is-active team-cli-tracker
sudo journalctl -u team-cli-tracker -n 50 --no-pager
curl -s http://127.0.0.1:4101/metrics
```

### 5) Обновление
На каждой ноде:
```bash
cd /opt/team-cli-tracker
git fetch origin
git checkout main
git pull --ff-only origin main
sudo systemctl restart team-cli-tracker
```

### 6) Откат (быстрый откат одной ноды)
```bash
cd /opt/team-cli-tracker
git log --oneline -n 5
git checkout <previous_commit_sha>
sudo systemctl restart team-cli-tracker
```

### 7) Если участников 2 или больше 3

#### Если участников 2
- Рекомендуется держать `3` voting nodes и добавить легкую witness-ноду.
- Причина: при 2 нодах Raft-кворум хрупкий (нет отказоустойчивости, риск split-brain).
- Если один пользователь админ, назначай его ноду `preferred leader` (приоритет), но не жестко фиксируй лидерство.
- Если админ-нода недоступна, выбор нового лидера должен продолжаться автоматически.

#### Если участников > 3
- Не все участники должны быть voting nodes.
- Рекомендуемый подход:
1. Держать `3` voting nodes при небольшой/средней нагрузке.
2. Переходить на `5` voting nodes при росте нагрузки/требований к доступности.
3. Остальных участников держать как client/non-voting nodes.

#### Правило кворума
- Использовать нечетное число voting nodes: `3`, `5`, `7`.
- Кворум: `N/2 + 1`.
- Не закреплять лидера жестко за одной ролью/нодой, иначе failover может сломаться.

#### Режим 5 нод
Используй шаблоны:
1. `deploy/cluster.node-1.env.example`
2. `deploy/cluster.node-2.env.example`
3. `deploy/cluster.node-3.env.example`
4. `deploy/cluster.node-4.env.example`
5. `deploy/cluster.node-5.env.example`

### 8) Protected transition проверка
Пример перехода через защищенный policy-контур:
```bash
go run ./cmd/node issue transition --project-id OPS --issue-id OPS-1 --from todo --to in_progress --policy-url http://127.0.0.1:4101
```

---

## EN

This runbook gives exact command order for `node-1`, `node-2`, `node-3`.

### 0) Prerequisites
- 3 Debian 13 VPS with SSH access.
- Nodes can reach each other in private network (Tailscale/ZeroTier recommended).
- Repo: `https://github.com/flakeman/team-cli-tracker`

### 1) On each node: clone
```bash
git clone https://github.com/flakeman/team-cli-tracker.git
cd team-cli-tracker
```

### 2) Node-specific config

#### node-1
```bash
cp deploy/cluster.node-1.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

#### node-2
```bash
cp deploy/cluster.node-2.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

#### node-3
```bash
cp deploy/cluster.node-3.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

### 3) Install and start systemd service
Run on each node:
```bash
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
```

### 4) Verify
Run on each node:
```bash
systemctl is-active team-cli-tracker
sudo journalctl -u team-cli-tracker -n 50 --no-pager
curl -s http://127.0.0.1:4101/metrics
```

### 5) Update rollout
On each node:
```bash
cd /opt/team-cli-tracker
git fetch origin
git checkout main
git pull --ff-only origin main
sudo systemctl restart team-cli-tracker
```

### 6) Rollback (single-node quick rollback)
```bash
cd /opt/team-cli-tracker
git log --oneline -n 5
git checkout <previous_commit_sha>
sudo systemctl restart team-cli-tracker
```

### 7) If Team Size Is 2 Or More Than 3

#### Team size = 2
- Recommended: keep `3` voting nodes and add one lightweight witness node.
- Why: Raft quorum with 2 nodes is fragile (no fault tolerance, split-brain risk).
- If one user is admin, set admin node as `preferred leader` (priority), not hard-locked leader.
- If admin node is down, non-admin leader election must continue automatically.

#### Team size > 3
- Users do not need to be voting nodes.
- Recommended:
1. Keep `3` voting nodes for small/medium load.
2. Move to `5` voting nodes for higher load/availability.
3. Keep remaining participants as client/non-voting nodes.

#### Quorum rule
- Use odd number of voting nodes: `3`, `5`, `7`.
- Quorum is `N/2 + 1`.
- Avoid hard pinning leader to one node role, otherwise failover can break.

#### 5-node mode
Use templates:
1. `deploy/cluster.node-1.env.example`
2. `deploy/cluster.node-2.env.example`
3. `deploy/cluster.node-3.env.example`
4. `deploy/cluster.node-4.env.example`
5. `deploy/cluster.node-5.env.example`

### 8) Protected transition check
Example transition via protected policy path:
```bash
go run ./cmd/node issue transition --project-id OPS --issue-id OPS-1 --from todo --to in_progress --policy-url http://127.0.0.1:4101
```
