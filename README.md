# team-cli-tracker

Keywords: kanban, task-tracker, self-hosted, distributed, raft, cli, golang, wireguard, audit-log, master-api, webhook, attachments, decentralized, p2p, devops

## RU

Децентрализованный CLI Kanban-трекер для команд, работающих с разных компьютеров, включая сценарии с динамическими домашними IP.

### Для чего этот проект
- Командный трекинг задач из терминала (Linux/PowerShell/macOS).
- Приватная peer-to-peer совместная работа без обязательного статического публичного IP.
- Подписанный аудит изменений задач.
- Ролевой контроль переходов по workflow (`admin`, `lead`, `dev`, `qa`, `viewer`).
- Хранение файлов-вложений в S3/MinIO (бинарные данные), с метаданными и checksum в event log.

### Целевая архитектура
- Приватный P2P-оверлей: Tailscale/ZeroTier + libp2p.
- Подписанный журнал событий по каждому проекту.
- CRDT-слияние для совместно редактируемых полей.
- Контур авторитетных решений на Raft для защищенных действий (RBAC/workflow policy).

### Структура репозитория
- `cmd/node` - точка входа процесса ноды.
- `internal/events` - модель подписанных событий.
- `internal/sync` - примитивы синхронизации/vector clock.
- `configs` - примеры конфигурации.
- `docs` - SDD, roadmap, runbook'и деплоя.

### Текущее состояние
- Закоммичен каркас MVP.
- Определены SDD и roadmap.
- Следующая веха: первый синхронизируемый кластер из 3 нод.

### SSH Topology (3 VPS)
- `srv1-22221 = srv1.example.internal` (SSH port `22`)
- `srv2-22222 = srv2.example.internal` (SSH port `22`)
- `srv3-22223 = srv3.example.internal` (SSH port `22`)

### Быстрый старт (локально)
```bash
go test ./...
go run ./cmd/node
```

### Примеры CLI

#### Пример проекции карточки задачи (JSON, не отдельный файл)
```json
{
  "id": "OPS-104",
  "type": "bug",
  "summary": "Fix timeout in billing worker",
  "status": "in_progress",
  "priority": "high",
  "assignee_id": "vladimir",
  "due_date": "2026-02-15",
  "updated_at": "2026-02-12T16:40:00Z"
}
```
Источник данных:
- Задачи хранятся как события в append-only event log (`data-dir`), а не как отдельные JSON-файлы.
- S3/MinIO используется для бинарных вложений; в event log хранятся метаданные и хеши вложений.

#### Пример проекции карточки с вложением (S3-backed)
```json
{
  "id": "OPS-104",
  "summary": "Fix timeout in billing worker",
  "status": "in_progress",
  "attachments": [
    {
      "id": "OPS-104-1739465000000000000",
      "filename": "trace.log",
      "storage_backend": "s3",
      "checksum_sha256": "9f86d081884c7d659a2feaa0c55ad015..."
    }
  ]
}
```
Минимальный flow загрузки файла:
```bash
# 1) получить upload_url + attachment_id
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-104","filename":"trace.log","content_type":"text/plain","size_bytes":1234}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/initiate"

# 2) загрузить байты в upload_url (S3/MinIO)
curl -X PUT --data-binary @./trace.log "<upload_url>"

# 3) зафиксировать вложение в event log
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-104","attachment_id":"<attachment_id>","filename":"trace.log","content_type":"text/plain","checksum_sha256":"<sha256>"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/complete"
```

#### Пример отображения доски (терминал)
```text
Project: OPS  Revision: 7
Assignee WIP limit: 3
Total issues: 7  Open: 5  Done: 2
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
| To Do [2]                        | In Progress [1/7]                | Code Review [1/7]                | Testing [1/7]                    | Done [2/7]                       |
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
| OPS-120 Add audit logs @unassign | OPS-104 Fix billing timeout @vla | OPS-101 Add health endpoint @lea | OPS-115 UI regression checks @qa | OPS-097 Update runbook @olga     |
| OPS-130 Add SLA reminder hooks @ |                                  |                                  |                                  | OPS-099 DB migration cleanup @dm |
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
```

#### Операторский quickstart (deploy -> users -> tasks -> files -> export)
```bash
# 1) Развернуть 3 ноды (на каждой ноде)
cp deploy/cluster.node-1.env.example deploy/cluster.env   # node-2/node-3: соответствующий файл
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
cd /opt/team-cli-tracker && go build -o node ./cmd/node
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
systemctl is-active team-cli-tracker

# 2) Добавить/проверить ноды доверия (из admin ноды)
go run ./cmd/node trust invite --node-id node-2 --ttl-sec 3600
go run ./cmd/node trust use-invite --node-id node-2 --token <invite-token>
go run ./cmd/node trust list

# 3) Добавить пользователей и роли
go run ./cmd/node team onboard --user-id lead1 --role lead
go run ./cmd/node team onboard --user-id dev1 --role dev
go run ./cmd/node team onboard --user-id qa1 --role qa
go run ./cmd/node auth issue --user-id dev1 --role dev --ttl-sec 86400

# 4) Создать и двигать задачи
go run ./cmd/node issue create --project-id OPS --issue-id OPS-200 --summary "Deploy check"
go run ./cmd/node issue transition --project-id OPS --issue-id OPS-200 --from todo --to in_progress --policy-url http://127.0.0.1:4101
go run ./cmd/node issue comment --project-id OPS --issue-id OPS-200 --text "Started"

# 5) Файлы в S3/MinIO (initiate -> upload -> complete)
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-200","filename":"trace.log","content_type":"text/plain","size_bytes":1234}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/initiate"
curl -X PUT --data-binary @./trace.log "<upload_url>"
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-200","attachment_id":"<attachment_id>","filename":"trace.log","content_type":"text/plain","checksum_sha256":"<sha256>"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/complete"

# 6) Выгрузка аудита (локальная и кластерная)
go run ./cmd/node audit export --all --format jsonl
go run ./cmd/node audit export --from 2026-02-15T00:00:00Z --to 2026-02-15T23:59:59Z --user admin1,dev1 --format csv
go run ./cmd/node audit cluster-export --data-dir ./data --self-node-id srv1 --peers https://srv2.example.internal:4101,https://srv3.example.internal:4101 --auth-token admin-token --all --format csv
```

#### Примеры команд
```bash
# запустить ноду
go run ./cmd/node

# отрисовать доску из API трекера
go run ./cmd/node board --project-id OPS

# включить архивные задачи в board output
go run ./cmd/node board --project-id OPS --include-archived

# отрисовать только свои задачи
go run ./cmd/node board --project-id OPS --view mine --assignee-id vova

# быстрая сводка по количеству задач в столбцах
go run ./cmd/node board --project-id OPS --counts-only

# интерактивная сессия доски (команды внутри: help/create/move/comment/view/counts/table/pause/resume/quit)
# автообновление можно выключить: --interactive-refresh 0s
# protected mode for interactive commands (move/create/comment via policy/API path):
go run ./cmd/node board --project-id OPS --interactive --interactive-refresh 0s --interactive-policy-url http://127.0.0.1:4101

# вывести машиночитаемый JSON
go run ./cmd/node board --project-id OPS -format json

# запустить sync-узел
go run ./cmd/node serve --project-id OPS --listen :4101 --peers "http://127.0.0.1:4102"

# миграция схемы event-store
go run ./cmd/node storage migrate --data-dir ./data

go run ./cmd/node storage enable-encryption --data-dir ./data
go run ./cmd/node storage rotate-key --data-dir ./data
go run ./cmd/node storage verify-integrity --data-dir ./data

# защищенный режим (TLS + token auth)
go run ./cmd/node serve --project-id OPS --listen :4101 \
  --tls-cert ./certs/node.pem --tls-key ./certs/node-key.pem \
  --auth-enabled --auth-tokens-json '{"admin-token":{"user_id":"u1","role":"admin","active":true}}' \
  --peer-token admin-token \
  --rate-limit-per-min 120 \
  --rate-limit-sensitive-per-min 30

# env-переменные для лимитов:
# RATE_LIMIT_PER_MIN=120
# RATE_LIMIT_SENSITIVE_PER_MIN=30

# жизненный цикл команды
go run ./cmd/node team onboard --user-id dev1 --role dev
go run ./cmd/node team role-change --user-id dev1 --role lead
go run ./cmd/node team offboard --project-id OPS --user-id dev1

# archive lifecycle
go run ./cmd/node issue archive --project-id OPS --issue-id OPS-101
go run ./cmd/node issue unarchive --project-id OPS --issue-id OPS-101

# доверенные ноды
go run ./cmd/node trust invite --node-id node-2 --ttl-sec 3600
go run ./cmd/node trust use-invite --node-id node-2 --token <invite-token>
go run ./cmd/node trust revoke --node-id node-2

# аудит: выгрузка и проверка целостности
go run ./cmd/node audit export --all --format jsonl
go run ./cmd/node audit export --from 2026-02-12T00:00:00Z --to 2026-02-12T23:59:59Z --user vova,qa --format csv --limit 100 --cursor 0
go run ./cmd/node audit cluster-export --data-dir ./data --self-node-id srv1 --peers https://srv2.example.internal:4101,https://srv3.example.internal:4101 --auth-token admin-token --all --format csv --limit 200 --insecure-tls
go run ./cmd/node audit verify-integrity

# API выгрузка аудита (admin/lead token)
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/security/audit/export?from=2026-02-12T00:00:00Z&to=2026-02-12T23:59:59Z&user=vova,qa&limit=100&cursor=0"

# Master API (v1 namespace, совместим с текущими endpoint'ами)
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/api/v1/team/list"
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/api/v1/security/audit/export?all=1"
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -H "X-Request-Id: req-001" \
  -d '{"project_id":"OPS","issue_id":"OPS-101","from":"todo","to":"in_progress"}' \
  "http://127.0.0.1:4101/api/v1/issue/transition"
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -H "X-Request-Id: req-002" \
  -d '{"project_id":"OPS","issue_id":"OPS-101"}' \
  "http://127.0.0.1:4101/api/v1/issue/archive"

# важно: mutating master endpoints (/api/v1/master/*) требуют X-Request-Id для идемпотентности
# audit хранится локально на каждой ноде (security_audit.jsonl); единая картина по кластеру:
curl -k -H "Authorization: Bearer <token>" \
  "https://127.0.0.1:4101/api/v1/master/audit/cluster-export?peers=https://srv2.example.internal:4101,https://srv3.example.internal:4101&peer_token=admin-token&insecure_tls=true&include_self=true&limit=200"

# Attachments (MVP)
# A) quick link-mode: add/list/open/remove
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-101","url":"https://files.example.com/spec.pdf","title":"Spec PDF"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/add"
curl -H "Authorization: Bearer <token>" \
  "http://127.0.0.1:4101/api/v1/issue/attachment/list?project_id=OPS&issue_id=OPS-101"
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-101","attachment_id":"OPS-101-1739465000000000000"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/open"

# B) file upload flow: initiate -> upload -> complete -> open
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-101","filename":"spec.pdf","content_type":"application/pdf","size_bytes":12345,"title":"Spec PDF"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/initiate"
# response has: attachment_id + upload_url

curl -X PUT --data-binary @./spec.pdf "<upload_url_from_initiate>"

# checksum (linux/mac):
# SHA=$(sha256sum ./spec.pdf | awk '{print $1}')

curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-101","attachment_id":"<attachment_id>","filename":"spec.pdf","content_type":"application/pdf","checksum_sha256":"<sha256>"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/complete"

# verify attachment integrity (re-hash from storage and compare)
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-101","attachment_id":"<attachment_id>"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/verify"
# note: URL-only attachments are marked integrity_verified=false and are not binary-verifiable

# verify all attachments in project (or pass issue_id to scope)
curl -H "Authorization: Bearer <token>" \
  "http://127.0.0.1:4101/api/v1/issue/attachment/verify-all?project_id=OPS"

# webhooks: create/list/test/revoke
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"url":"http://127.0.0.1:8088/hook","events":["issue.attachment.added","issue.attachment.removed"]}' \
  "http://127.0.0.1:4101/api/v1/webhooks/create"
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/api/v1/webhooks/list"

# Attachment backend selection on server:
# local (default): --attachment-backend local
# s3/minio:
#   --attachment-backend s3
#   --attachment-s3-endpoint 127.0.0.1:9000
#   --attachment-s3-bucket team-cli-attachments
#   --attachment-s3-access-key minioadmin
#   --attachment-s3-secret-key minioadmin
#   --attachment-s3-region us-east-1
#   --attachment-s3-secure=false
# optional periodic integrity scan:
#   --attachment-verify-interval 1h
# metrics fields:
#   attachment_verify_last_at
#   attachment_verify_checked
#   attachment_verify_mismatch
#   attachment_verify_errors
```

### Документация
- Start here: `docs/INDEX.md`
- Architecture: `docs/SDD-01-decentralized-kanban.md`
- Roadmap: `docs/ROADMAP.md`
- Deploy scripts: `deploy/README.md`

### Operations
- 3-node deploy: `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
- Isolated WG + HA: `docs/DEPLOYMENT-ISOLATED-WG-HA.md`
- Release checklist: `docs/RELEASE-CHECKLIST.md`
- Rollback runbook: `docs/ROLLBACK-RUNBOOK.md`
- Security runbook: `docs/SECURITY-RUNBOOK.md`

### Community
- License: `LICENSE`
- Contributing guide: `CONTRIBUTING.md`
- Code of Conduct: `CODE_OF_CONDUCT.md`
- Security policy: `SECURITY.md`

### Текущий фокус
1. Production-стабильность 3-ноды (`systemd`, secure mode, health/smoke).
2. Master API как единая control-plane точка (issues/team/trust/governance/auth/audit).
3. Сквозной аудит с кластерной выгрузкой (`master/audit/cluster-export`).
4. Хранилище файлов: S3/MinIO для бинарных вложений; event log хранит только метаданные и checksum.
5. Единая точка входа и HA-профили в изолированном контуре (WG + gateway/VIP).

---

## EN

Decentralized CLI Kanban tracker for teams working from different computers, including dynamic home IP scenarios.

### What This Project Is For
- Team task tracking from terminal (Linux/PowerShell/macOS).
- Private peer-to-peer collaboration without mandatory static public IP.
- Signed audit trail for task changes.
- Role-based workflow control for transitions (`admin`, `lead`, `dev`, `qa`, `viewer`).
- Attachment binaries stored in S3/MinIO, while metadata and checksums remain in the event log.

### Core Architecture (Target)
- Private P2P overlay: Tailscale/ZeroTier + libp2p.
- Signed event log per project.
- CRDT merge for collaborative fields.
- Raft authority path for protected actions (RBAC/workflow policy).

### Repository Structure
- `cmd/node` - node process entrypoint.
- `internal/events` - signed event model.
- `internal/sync` - sync/vector clock primitives.
- `configs` - configuration examples.
- `docs` - SDD, roadmap, deployment runbooks.

### Current State
- MVP bootstrap skeleton is committed.
- SDD and roadmap are defined.
- Next milestone: first 3-node syncable cluster.

### Quick Start (Local)
```bash
go test ./...
go run ./cmd/node
```

### CLI Examples

#### Example Task Projection (JSON, not a stored file)
```json
{
  "id": "OPS-104",
  "type": "bug",
  "summary": "Fix timeout in billing worker",
  "status": "in_progress",
  "priority": "high",
  "assignee_id": "vladimir",
  "due_date": "2026-02-15",
  "updated_at": "2026-02-12T16:40:00Z"
}
```
Data source:
- Tasks are derived from append-only event log entries (`data-dir`), not stored as standalone JSON files.
- S3/MinIO stores attachment binaries; event log stores attachment metadata and checksums.

#### Example Task Projection With Attachment (S3-backed)
```json
{
  "id": "OPS-104",
  "summary": "Fix timeout in billing worker",
  "status": "in_progress",
  "attachments": [
    {
      "id": "OPS-104-1739465000000000000",
      "filename": "trace.log",
      "storage_backend": "s3",
      "checksum_sha256": "9f86d081884c7d659a2feaa0c55ad015..."
    }
  ]
}
```

#### Example Board View (Terminal)
```text
Project: OPS  Revision: 7
Assignee WIP limit: 3
Total issues: 7  Open: 5  Done: 2
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
| To Do [2]                        | In Progress [1/7]                | Code Review [1/7]                | Testing [1/7]                    | Done [2/7]                       |
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
| OPS-120 Add audit logs @unassign | OPS-104 Fix billing timeout @vla | OPS-101 Add health endpoint @lea | OPS-115 UI regression checks @qa | OPS-097 Update runbook @olga     |
| OPS-130 Add SLA reminder hooks @ |                                  |                                  |                                  | OPS-099 DB migration cleanup @dm |
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
```

#### Operator Quickstart (deploy -> users -> tasks -> files -> export)
```bash
# 1) Deploy 3 nodes (run on each node)
cp deploy/cluster.node-1.env.example deploy/cluster.env   # use node-2/node-3 template accordingly
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
cd /opt/team-cli-tracker && go build -o node ./cmd/node
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
systemctl is-active team-cli-tracker

# 2) Add/check trusted nodes (from admin node)
go run ./cmd/node trust invite --node-id node-2 --ttl-sec 3600
go run ./cmd/node trust use-invite --node-id node-2 --token <invite-token>
go run ./cmd/node trust list

# 3) Add users and roles
go run ./cmd/node team onboard --user-id lead1 --role lead
go run ./cmd/node team onboard --user-id dev1 --role dev
go run ./cmd/node team onboard --user-id qa1 --role qa
go run ./cmd/node auth issue --user-id dev1 --role dev --ttl-sec 86400

# 4) Create and move tasks
go run ./cmd/node issue create --project-id OPS --issue-id OPS-200 --summary "Deploy check"
go run ./cmd/node issue transition --project-id OPS --issue-id OPS-200 --from todo --to in_progress --policy-url http://127.0.0.1:4101
go run ./cmd/node issue comment --project-id OPS --issue-id OPS-200 --text "Started"

# 5) Files in S3/MinIO (initiate -> upload -> complete)
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-200","filename":"trace.log","content_type":"text/plain","size_bytes":1234}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/initiate"
curl -X PUT --data-binary @./trace.log "<upload_url>"
curl -X POST -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"project_id":"OPS","issue_id":"OPS-200","attachment_id":"<attachment_id>","filename":"trace.log","content_type":"text/plain","checksum_sha256":"<sha256>"}' \
  "http://127.0.0.1:4101/api/v1/issue/attachment/complete"

# 6) Audit export (local and cluster)
go run ./cmd/node audit export --all --format jsonl
go run ./cmd/node audit export --from 2026-02-15T00:00:00Z --to 2026-02-15T23:59:59Z --user admin1,dev1 --format csv
go run ./cmd/node audit cluster-export --data-dir ./data --self-node-id srv1 --peers https://srv2.example.internal:4101,https://srv3.example.internal:4101 --auth-token admin-token --all --format csv
```

#### Example Commands
```bash
# run node
go run ./cmd/node

# render board from tracker API
go run ./cmd/node board --project-id OPS

# render only my tasks
go run ./cmd/node board --project-id OPS --view mine --assignee-id vova

# counts-only summary
go run ./cmd/node board --project-id OPS --counts-only

# interactive board session (in-session commands: help/create/move/comment/view/counts/table/pause/resume/quit)
# auto-refresh can be disabled: --interactive-refresh 0s
# protected mode for interactive commands (move/create/comment via policy/API path):
go run ./cmd/node board --project-id OPS --interactive --interactive-refresh 0s --interactive-policy-url http://127.0.0.1:4101

# machine-readable output
go run ./cmd/node board --project-id OPS -format json

# run sync node
go run ./cmd/node serve --project-id OPS --listen :4101 --peers "http://127.0.0.1:4102"

# run event-store schema migration hooks
go run ./cmd/node storage migrate --data-dir ./data

go run ./cmd/node storage enable-encryption --data-dir ./data
go run ./cmd/node storage rotate-key --data-dir ./data
go run ./cmd/node storage verify-integrity --data-dir ./data

# secure mode (TLS + token auth)
go run ./cmd/node serve --project-id OPS --listen :4101 \
  --tls-cert ./certs/node.pem --tls-key ./certs/node-key.pem \
  --auth-enabled --auth-tokens-json '{"admin-token":{"user_id":"u1","role":"admin","active":true}}' \
  --peer-token admin-token \
  --rate-limit-per-min 120 \
  --rate-limit-sensitive-per-min 30

# env vars for limits:
# RATE_LIMIT_PER_MIN=120
# RATE_LIMIT_SENSITIVE_PER_MIN=30

# team lifecycle
go run ./cmd/node team onboard --user-id dev1 --role dev
go run ./cmd/node team role-change --user-id dev1 --role lead
go run ./cmd/node team offboard --project-id OPS --user-id dev1

# trusted node management
go run ./cmd/node trust invite --node-id node-2 --ttl-sec 3600
go run ./cmd/node trust use-invite --node-id node-2 --token <invite-token>
go run ./cmd/node trust revoke --node-id node-2

# audit export and integrity
go run ./cmd/node audit export --all --format jsonl
go run ./cmd/node audit export --from 2026-02-12T00:00:00Z --to 2026-02-12T23:59:59Z --user vova,qa --format csv --limit 100 --cursor 0
go run ./cmd/node audit cluster-export --data-dir ./data --self-node-id srv1 --peers https://srv2.example.internal:4101,https://srv3.example.internal:4101 --auth-token admin-token --all --format csv --limit 200 --insecure-tls
go run ./cmd/node audit verify-integrity

# audit export API (admin/lead token)
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/security/audit/export?from=2026-02-12T00:00:00Z&to=2026-02-12T23:59:59Z&user=vova,qa&limit=100&cursor=0"
curl -k -H "Authorization: Bearer <token>" "https://127.0.0.1:4101/api/v1/master/audit/cluster-export?peers=https://srv2.example.internal:4101,https://srv3.example.internal:4101&peer_token=admin-token&insecure_tls=true&include_self=true&limit=200"

# note:
# - security_audit.jsonl is local per node;
# - use master/audit/cluster-export for merged cluster view;
# - mutating master endpoints require X-Request-Id.
```

### Project Hub
- Quick docs index: `docs/INDEX.md`
- Architecture + SDD stream: `docs/SDD-01-decentralized-kanban.md`
- Roadmap: `docs/ROADMAP.md`
- Release checklist: `docs/RELEASE-CHECKLIST.md`
- Latest smoke evidence: `docs/SMOKE-SECURE-SYSTEMD-3VPS-2026-02-15.md`
- Master API smoke: `docs/SMOKE-MASTER-API-3VPS-2026-02-15.md`
- Secure deploy (3 nodes): `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
- Isolated WG + HA entrypoint: `docs/DEPLOYMENT-ISOLATED-WG-HA.md`
- Deploy scripts: `deploy/README.md`








