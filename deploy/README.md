# Deploy Toolkit

## RU

Эта папка содержит вспомогательные скрипты для тестового кластера `team-cli-tracker` на Ubuntu 24 / Debian 13.

### Файлы
- `cluster.env.example` - общий шаблон переменных.
- `cluster.node-1.env.example` - готовый шаблон для `srv1` (node-1).
- `cluster.node-2.env.example` - готовый шаблон для `srv2` (node-2).
- `cluster.node-3.env.example` - готовый шаблон для `srv3` (node-3).
- `cluster.node-4.env.example` - готовый шаблон для node-4 (режим 5 нод).
- `cluster.node-5.env.example` - готовый шаблон для node-5 (режим 5 нод).
- `gateway.env.example` - шаблон параметров единой точки входа.
- `wireguard.env.example` - шаблон WireGuard-параметров ноды.
- `minio.env.example` - шаблон MinIO (S3) параметров ноды.
- `keepalived.env.example` - шаблон VRRP/keepalived для VIP единой точки входа.
- `bootstrap-debian13.sh` - базовая подготовка ОС + установка Go + clone/update репозитория.
- `gen-node-config.sh` - создает `configs/node-<id>.yaml` из env-переменных.
- `install-systemd-service.sh` - ставит и запускает `team-cli-tracker.service`.
- `install-gateway-caddy.sh` - ставит Caddy reverse-proxy как единую точку входа.
- `install-gateway-cloudflared.sh` - ставит cloudflared для публикации gateway при dynamic IP.
- `install-wireguard.sh` - ставит и включает WireGuard (`wg-quick@wg0`) из env-шаблона.
- `install-minio.sh` - ставит MinIO + `mc` client.
- `install-minio-systemd.sh` - ставит `minio.service` (distributed mode).
- `bootstrap-minio.sh` - создаёт bucket/policy для вложений.
- `minio-healthcheck.sh` - проверяет live/ready и доступ к bucket.
- `install-keepalived.sh` - ставит keepalived и VRRP VIP failover для `board.internal`.
- `pki-manage.sh` - базовая автоматизация PKI: init CA, issue/revoke node cert, CRL.
- `RUNBOOK-QUICKSTART-3NODES.md` - точная последовательность команд по нодам.

### Типовой порядок
1. Скопировать `cluster.env.example` в `cluster.env` и заполнить значения.
2. При наличии внутреннего DNS указать `INTERNAL_DNS` (по умолчанию `172.16.254.254`).
3. Запустить `bootstrap-debian13.sh` на каждой VPS.
4. Запустить `gen-node-config.sh` на каждой VPS с параметрами своей ноды.
5. Запустить `install-systemd-service.sh` на каждой VPS.
6. Проверить статус сервиса и логи.
7. (Опционально) поднять единую точку входа:
8. Скопировать `gateway.env.example` в `gateway.env`.
9. Запустить `install-gateway-caddy.sh` на gateway-хосте.
10. Для dynamic IP опубликовать gateway через `install-gateway-cloudflared.sh`.
11. Для изолированного контура без внешних SaaS:
12. Настроить WireGuard на всех нодах через `wireguard.env` + `install-wireguard.sh`.
13. Для S3 backend:
14. Скопировать `minio.env.example` в `minio.env`.
15. Запустить `install-minio.sh` + `install-minio-systemd.sh` на всех нодах.
16. Выполнить `bootstrap-minio.sh` (bucket/policy) и `minio-healthcheck.sh`.
17. Для единой точки входа через VIP (рекомендуется для isolated):
18. Скопировать `keepalived.env.example` в `keepalived.env` на каждой ноде.
19. Проставить `KA_STATE/KA_PRIORITY` (MASTER на одной ноде, BACKUP на остальных).
20. Запустить `install-keepalived.sh` на всех нодах.

---

## EN

This folder contains helper scripts for an Ubuntu 24 / Debian 13 `team-cli-tracker` test cluster.

### Files
- `cluster.env.example` - common variables template.
- `cluster.node-1.env.example` - ready config template for `srv1` (node-1).
- `cluster.node-2.env.example` - ready config template for `srv2` (node-2).
- `cluster.node-3.env.example` - ready config template for `srv3` (node-3).
- `cluster.node-4.env.example` - ready config template for node-4 (5-node mode).
- `cluster.node-5.env.example` - ready config template for node-5 (5-node mode).
- `gateway.env.example` - single-entrypoint config template.
- `wireguard.env.example` - WireGuard node config template.
- `minio.env.example` - MinIO (S3) node config template.
- `keepalived.env.example` - VRRP/keepalived template for VIP entrypoint.
- `bootstrap-debian13.sh` - base OS setup + Go install + repo clone/update.
- `gen-node-config.sh` - creates `configs/node-<id>.yaml` from env values.
- `install-systemd-service.sh` - installs and starts `team-cli-tracker.service`.
- `install-gateway-caddy.sh` - installs Caddy reverse-proxy as single entrypoint.
- `install-gateway-cloudflared.sh` - installs cloudflared for dynamic-IP publication.
- `install-wireguard.sh` - installs and enables WireGuard (`wg-quick@wg0`) from env template.
- `install-minio.sh` - installs MinIO + `mc` client.
- `install-minio-systemd.sh` - installs `minio.service` (distributed mode).
- `bootstrap-minio.sh` - creates bucket/policy for attachments.
- `minio-healthcheck.sh` - validates live/ready and bucket access.
- `install-keepalived.sh` - installs keepalived and VRRP VIP failover for `board.internal`.
- `pki-manage.sh` - basic PKI automation: init CA, issue/revoke node cert, CRL.
- `RUNBOOK-QUICKSTART-3NODES.md` - exact per-node command sequence.

### Typical Flow
1. Copy `cluster.env.example` to `cluster.env` and fill values.
2. Set `INTERNAL_DNS` for private resolver (default `172.16.254.254`) if required.
3. Run `bootstrap-debian13.sh` on each VPS.
4. Run `gen-node-config.sh` on each VPS with node-specific IDs/ports/peers.
5. Run `install-systemd-service.sh` on each VPS.
6. Validate service and logs.
7. (Optional) enable single entrypoint:
8. Copy `gateway.env.example` to `gateway.env`.
9. Run `install-gateway-caddy.sh` on gateway host.
10. For dynamic IP, publish gateway with `install-gateway-cloudflared.sh`.
11. For isolated no-SaaS contour:
12. Configure WireGuard on all nodes via `wireguard.env` + `install-wireguard.sh`.
13. For S3 backend:
14. Copy `minio.env.example` to `minio.env`.
15. Run `install-minio.sh` + `install-minio-systemd.sh` on all nodes.
16. Run `bootstrap-minio.sh` and `minio-healthcheck.sh`.
17. For VIP single entrypoint (recommended for isolated contour):
18. Copy `keepalived.env.example` to `keepalived.env` on each node.
19. Set `KA_STATE/KA_PRIORITY` (MASTER on one node, BACKUP on others).
20. Run `install-keepalived.sh` on all nodes.
