# team-cli-tracker

Децентрализованный CLI Kanban-трекер для команд, работающих с разных компьютеров, включая сценарии с динамическими домашними IP.

## Для чего этот проект
- Командный трекинг задач из терминала (Linux/PowerShell/macOS).
- Приватная peer-to-peer совместная работа без обязательного статического публичного IP.
- Подписанный аудит изменений задач.
- Ролевой контроль переходов по workflow (`admin`, `lead`, `dev`, `qa`, `viewer`).

## Целевая архитектура
- Приватный P2P-оверлей: Tailscale/ZeroTier + libp2p.
- Подписанный журнал событий по каждому проекту.
- CRDT-слияние для совместно редактируемых полей.
- Контур авторитетных решений на Raft для защищенных действий (RBAC/workflow policy).

## Структура репозитория
- `cmd/node` - точка входа процесса ноды.
- `internal/events` - модель подписанных событий.
- `internal/sync` - примитивы синхронизации/vector clock.
- `configs` - примеры конфигурации.
- `docs` - SDD, roadmap, runbook'и деплоя.

## Текущее состояние
- Закоммичен каркас MVP.
- Определены SDD и roadmap.
- Следующая веха: первый синхронизируемый кластер из 3 нод.

## Быстрый старт (локально)
```bash
go test ./...
go run ./cmd/node
```

## Примеры CLI

### Пример карточки задачи (JSON)
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

### Пример отображения доски (терминал)
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

### Примеры команд
```bash
# запустить ноду
go run ./cmd/node

# отрисовать доску из API трекера
go run ./cmd/kanban-cli -base-url http://localhost:8084 -project-id OPS

# вывести машиночитаемый JSON
go run ./cmd/kanban-cli -base-url http://localhost:8084 -project-id OPS -format json
```

## Документация
- Спецификация архитектуры: `docs/SDD-01-decentralized-kanban.md`
- Дорожная карта: `docs/ROADMAP.md`
- Гайд деплоя на 3 ноды: `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
- Скрипты деплоя: `deploy/README.md`

## Ближайший план
1. Идентификация ноды и append подписанных событий.
2. P2P-discovery пиров и синхронизация состояния.
3. Базовые команды задач: create, transition, comment.
4. CLI-рендер доски из реплицированного состояния.
5. Проверка переходов через защищенный Raft-контур.
