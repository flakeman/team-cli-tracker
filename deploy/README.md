# Deploy Toolkit

## RU

Эта папка содержит вспомогательные скрипты для тестового кластера `team-cli-tracker` на Debian 13.

### Файлы
- `cluster.env.example` - общий шаблон переменных.
- `cluster.node-1.env.example` - готовый шаблон для node-1.
- `cluster.node-2.env.example` - готовый шаблон для node-2.
- `cluster.node-3.env.example` - готовый шаблон для node-3.
- `cluster.node-4.env.example` - готовый шаблон для node-4 (режим 5 нод).
- `cluster.node-5.env.example` - готовый шаблон для node-5 (режим 5 нод).
- `bootstrap-debian13.sh` - базовая подготовка ОС + установка Go + clone/update репозитория.
- `gen-node-config.sh` - создает `configs/node-<id>.yaml` из env-переменных.
- `install-systemd-service.sh` - ставит и запускает `team-cli-tracker.service`.
- `RUNBOOK-QUICKSTART-3NODES.md` - точная последовательность команд по нодам.

### Типовой порядок
1. Скопировать `cluster.env.example` в `cluster.env` и заполнить значения.
2. Запустить `bootstrap-debian13.sh` на каждой VPS.
3. Запустить `gen-node-config.sh` на каждой VPS с параметрами своей ноды.
4. Запустить `install-systemd-service.sh` на каждой VPS.
5. Проверить статус сервиса и логи.

---

## EN

This folder contains helper scripts for a Debian 13 `team-cli-tracker` test cluster.

### Files
- `cluster.env.example` - common variables template.
- `cluster.node-1.env.example` - ready config template for node-1.
- `cluster.node-2.env.example` - ready config template for node-2.
- `cluster.node-3.env.example` - ready config template for node-3.
- `cluster.node-4.env.example` - ready config template for node-4 (5-node mode).
- `cluster.node-5.env.example` - ready config template for node-5 (5-node mode).
- `bootstrap-debian13.sh` - base OS setup + Go install + repo clone/update.
- `gen-node-config.sh` - creates `configs/node-<id>.yaml` from env values.
- `install-systemd-service.sh` - installs and starts `team-cli-tracker.service`.
- `RUNBOOK-QUICKSTART-3NODES.md` - exact per-node command sequence.

### Typical Flow
1. Copy `cluster.env.example` to `cluster.env` and fill values.
2. Run `bootstrap-debian13.sh` on each VPS.
3. Run `gen-node-config.sh` on each VPS with node-specific IDs/ports/peers.
4. Run `install-systemd-service.sh` on each VPS.
5. Validate service and logs.
