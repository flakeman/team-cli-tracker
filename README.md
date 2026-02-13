# team-cli-tracker

## RU

Децентрализованный CLI Kanban-трекер для команд, работающих с разных компьютеров, включая сценарии с динамическими домашними IP.

### Для чего этот проект
- Командный трекинг задач из терминала (Linux/PowerShell/macOS).
- Приватная peer-to-peer совместная работа без обязательного статического публичного IP.
- Подписанный аудит изменений задач.
- Ролевой контроль переходов по workflow (`admin`, `lead`, `dev`, `qa`, `viewer`).

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

### Быстрый старт (локально)
```bash
go test ./...
go run ./cmd/node
```

### Примеры CLI

#### Пример карточки задачи (JSON)
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

#### Примеры команд
```bash
# запустить ноду
go run ./cmd/node

# отрисовать доску из API трекера
go run ./cmd/node board --project-id OPS

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

# доверенные ноды
go run ./cmd/node trust invite --node-id node-2 --ttl-sec 3600
go run ./cmd/node trust use-invite --node-id node-2 --token <invite-token>
go run ./cmd/node trust revoke --node-id node-2

# аудит: выгрузка и проверка целостности
go run ./cmd/node audit export --all --format jsonl
go run ./cmd/node audit export --from 2026-02-12T00:00:00Z --to 2026-02-12T23:59:59Z --user vova,qa --format csv --limit 100 --cursor 0
go run ./cmd/node audit verify-integrity

# API выгрузка аудита (admin/lead token)
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/security/audit/export?from=2026-02-12T00:00:00Z&to=2026-02-12T23:59:59Z&user=vova,qa&limit=100&cursor=0"
```

### Документация
- Спецификация архитектуры: `docs/SDD-01-decentralized-kanban.md`
- SDD ближайшего этапа: `docs/SDD-02-near-term-execution-plan.md`
- SDD безопасности и жизненного цикла команды: `docs/SDD-03-security-and-team-lifecycle.md`
- SDD consensus-критичных мутаций: `docs/SDD-04-consensus-critical-mutations.md`
- SDD board UX, 3-VPS smoke и release baseline: `docs/SDD-05-board-smoke-and-release.md`
- SDD сквозного логирования и выгрузки аудита: `docs/SDD-06-end-to-end-audit-logging-and-export.md`
- План SDD-06: `docs/SDD-06-end-to-end-audit-logging-and-export.plan.md`
- Задачи SDD-06: `docs/SDD-06-end-to-end-audit-logging-and-export.tasks.md`
- SDD визуальной полировки board: `docs/SDD-07-board-visual-polish.md`
- План SDD-07: `docs/SDD-07-board-visual-polish.plan.md`
- Задачи SDD-07: `docs/SDD-07-board-visual-polish.tasks.md`
- SDD интерактивного board-режима: `docs/SDD-08-board-interactive-and-operator-flow.md`
- План SDD-08: `docs/SDD-08-board-interactive-and-operator-flow.plan.md`
- Задачи SDD-08: `docs/SDD-08-board-interactive-and-operator-flow.tasks.md`
- SDD подсчётов board и cadence interactive: `docs/SDD-09-board-counting-and-interactive-cadence.md`
- План SDD-09: `docs/SDD-09-board-counting-and-interactive-cadence.plan.md`
- Задачи SDD-09: `docs/SDD-09-board-counting-and-interactive-cadence.tasks.md`
- SDD parity interactive с protected policy flow: `docs/SDD-10-interactive-protected-policy-parity.md`
- План SDD-10: `docs/SDD-10-interactive-protected-policy-parity.plan.md`
- Задачи SDD-10: `docs/SDD-10-interactive-protected-policy-parity.tasks.md`
- Smoke interactive board (`main`, 3 VPS, 2026-02-13): `docs/SMOKE-BOARD-INTERACTIVE-3VPS-2026-02-13.md`
- Extended smoke (`main`, 3 VPS, 2026-02-13): `docs/SMOKE-EXTENDED-3VPS-2026-02-13.md`
- Smoke board live (`main`, 3 VPS, 2026-02-12): `docs/SMOKE-BOARD-LIVE-3VPS-2026-02-12.md`
- Smoke report (`main`, 3 VPS, 2026-02-12): `docs/SMOKE-MAIN-3VPS-2026-02-12.md`
- Changelog: `CHANGELOG.md`
- Release checklist: `docs/RELEASE-CHECKLIST.md`
- Release notes template: `docs/RELEASE-NOTES-TEMPLATE.md`
- Release notes draft (`v0.8.0`): `docs/releases-v0.8.0.md`
- Security runbook и incident checklist: `docs/SECURITY-RUNBOOK.md`
- Реестр технического долга: `docs/TECH-DEBT.md`
- Дорожная карта: `docs/ROADMAP.md`
- Гайд деплоя на 3 ноды: `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
- Скрипты деплоя: `deploy/README.md`

### Ближайший план
1. Идентификация ноды и append подписанных событий.
2. P2P-discovery пиров и синхронизация состояния.
3. Базовые команды задач: create, transition, comment.
4. CLI-рендер доски из реплицированного состояния.
5. Проверка переходов через защищенный Raft-контур.

---

## EN

Decentralized CLI Kanban tracker for teams working from different computers, including dynamic home IP scenarios.

### What This Project Is For
- Team task tracking from terminal (Linux/PowerShell/macOS).
- Private peer-to-peer collaboration without mandatory static public IP.
- Signed audit trail for task changes.
- Role-based workflow control for transitions (`admin`, `lead`, `dev`, `qa`, `viewer`).

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

#### Example Task Card (JSON)
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
go run ./cmd/node audit verify-integrity

# audit export API (admin/lead token)
curl -H "Authorization: Bearer <token>" "http://127.0.0.1:4101/security/audit/export?from=2026-02-12T00:00:00Z&to=2026-02-12T23:59:59Z&user=vova,qa&limit=100&cursor=0"
```

### Docs
- Architecture spec: `docs/SDD-01-decentralized-kanban.md`
- Near-term execution SDD: `docs/SDD-02-near-term-execution-plan.md`
- Security and team lifecycle SDD: `docs/SDD-03-security-and-team-lifecycle.md`
- Consensus-critical mutations SDD: `docs/SDD-04-consensus-critical-mutations.md`
- Board UX, 3-VPS smoke, and release baseline SDD: `docs/SDD-05-board-smoke-and-release.md`
- End-to-end audit logging and export SDD: `docs/SDD-06-end-to-end-audit-logging-and-export.md`
- SDD-06 plan: `docs/SDD-06-end-to-end-audit-logging-and-export.plan.md`
- SDD-06 tasks: `docs/SDD-06-end-to-end-audit-logging-and-export.tasks.md`
- Board visual polish SDD: `docs/SDD-07-board-visual-polish.md`
- SDD-07 plan: `docs/SDD-07-board-visual-polish.plan.md`
- SDD-07 tasks: `docs/SDD-07-board-visual-polish.tasks.md`
- Board interactive mode SDD: `docs/SDD-08-board-interactive-and-operator-flow.md`
- SDD-08 plan: `docs/SDD-08-board-interactive-and-operator-flow.plan.md`
- SDD-08 tasks: `docs/SDD-08-board-interactive-and-operator-flow.tasks.md`
- Board counting and interactive cadence SDD: `docs/SDD-09-board-counting-and-interactive-cadence.md`
- SDD-09 plan: `docs/SDD-09-board-counting-and-interactive-cadence.plan.md`
- SDD-09 tasks: `docs/SDD-09-board-counting-and-interactive-cadence.tasks.md`
- Interactive protected-policy parity SDD: `docs/SDD-10-interactive-protected-policy-parity.md`
- SDD-10 plan: `docs/SDD-10-interactive-protected-policy-parity.plan.md`
- SDD-10 tasks: `docs/SDD-10-interactive-protected-policy-parity.tasks.md`
- Interactive board smoke (`main`, 3 VPS, 2026-02-13): `docs/SMOKE-BOARD-INTERACTIVE-3VPS-2026-02-13.md`
- Extended smoke (`main`, 3 VPS, 2026-02-13): `docs/SMOKE-EXTENDED-3VPS-2026-02-13.md`
- Board live smoke (`main`, 3 VPS, 2026-02-12): `docs/SMOKE-BOARD-LIVE-3VPS-2026-02-12.md`
- Smoke report (`main`, 3 VPS, 2026-02-12): `docs/SMOKE-MAIN-3VPS-2026-02-12.md`
- Changelog: `CHANGELOG.md`
- Release checklist: `docs/RELEASE-CHECKLIST.md`
- Release notes template: `docs/RELEASE-NOTES-TEMPLATE.md`
- Release notes draft (`v0.8.0`): `docs/releases-v0.8.0.md`
- Security runbook and incident checklist: `docs/SECURITY-RUNBOOK.md`
- Technical debt register: `docs/TECH-DEBT.md`
- Delivery roadmap: `docs/ROADMAP.md`
- 3-node deploy guide: `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
- Deploy scripts: `deploy/README.md`

### Near-Term Plan
1. Node identity and signed event append.
2. P2P peer discovery and state sync.
3. Basic issue commands: create, transition, comment.
4. CLI board rendering from replicated state.
5. Raft-protected transition validation.







