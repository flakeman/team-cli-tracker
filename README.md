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
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
| To Do [2/20]                     | In Progress [1/8]                | Code Review [1/6]                | Testing [1/6]                    | Done [2/inf]                     |
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
go run ./cmd/kanban-cli -base-url http://localhost:8084 -project-id OPS

# вывести машиночитаемый JSON
go run ./cmd/kanban-cli -base-url http://localhost:8084 -project-id OPS -format json
```

### Документация
- Спецификация архитектуры: `docs/SDD-01-decentralized-kanban.md`
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
+----------------------------------+----------------------------------+----------------------------------+----------------------------------+----------------------------------+
| To Do [2/20]                     | In Progress [1/8]                | Code Review [1/6]                | Testing [1/6]                    | Done [2/inf]                     |
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
go run ./cmd/kanban-cli -base-url http://localhost:8084 -project-id OPS

# machine-readable output
go run ./cmd/kanban-cli -base-url http://localhost:8084 -project-id OPS -format json
```

### Docs
- Architecture spec: `docs/SDD-01-decentralized-kanban.md`
- Delivery roadmap: `docs/ROADMAP.md`
- 3-node deploy guide: `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
- Deploy scripts: `deploy/README.md`

### Near-Term Plan
1. Node identity and signed event append.
2. P2P peer discovery and state sync.
3. Basic issue commands: create, transition, comment.
4. CLI board rendering from replicated state.
5. Raft-protected transition validation.
