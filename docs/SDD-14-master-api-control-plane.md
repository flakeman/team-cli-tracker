# SDD-14: Master API Control Plane

## Status
- Proposed
- Date: 2026-02-15
- Revision: 1

## Context
Текущий API покрывает большую часть доменных операций, но отсутствует единый control-plane слой для централизованного управления системой "из одной точки".

## Goals
1. Ввести `Master API` как единый управленческий слой.
2. Дать консистентный namespace для orchestration операций.
3. Обеспечить полную parity с CLI и существующими API путями.
4. Сделать безопасный bootstrap для внешних интеграций.

## Non-Goals
1. Полная замена всех существующих endpoint в один релиз.
2. Миграция клиентов без backward compatibility.

## API Scope (v1)
1. Bootstrap:
- `GET /api/v1/master/health`
- `GET /api/v1/master/capabilities`

2. Control namespaces (phase rollout):
- `master/issues/*`
- `master/attachments/*`
- `master/team/*`
- `master/auth/*`
- `master/trust/*`
- `master/governance/*`
- `master/audit/*`
- `master/webhooks/*`

3. Cluster admin (phase rollout):
- `master/cluster/nodes`
- `master/cluster/health`
- `master/cluster/reconfigure`

## Security
1. RBAC минимум: `admin`, `lead` для control-plane.
2. Все master-вызовы пишутся в audit.
3. Идемпотентность мутаций через request-id (phase rollout).

## Acceptance Criteria
1. Bootstrap endpoints доступны и возвращают capability map.
2. Документация содержит roadmap миграции в master namespace.
3. Определены этапы перехода без breaking changes.
