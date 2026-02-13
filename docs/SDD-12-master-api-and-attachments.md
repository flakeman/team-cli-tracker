# SDD-12: Master API And File Attachments

## Status
- Proposed
- Date: 2026-02-13
- Revision: 1

## Context
Нужно перейти от частичного API к полному управлению системой через единый API-контур:
- управление участниками и ролями;
- выполнение всех действий с задачами;
- аудит и безопасность;
- интеграции.

Также требуется поддержка вложений (ссылки на файлы) с возможностью открытия файлов пользователями.

## Goals
1. Ввести `Master API` как единый контрольный слой для всех действий платформы.
2. Достичь API parity с CLI для задач, команды, trust, governance, auth, audit.
3. Добавить first-class support вложений к задачам.
4. Реализовать безопасное хранение файлов и доступ к ним по времени/ролям.

## Non-Goals
- Полноценный файловый CDN в рамках этого этапа.
- Полная DLP/антивирус-платформа (только точки расширения).

## Master API Scope
1. Issues
- `POST /api/v1/issues` create
- `POST /api/v1/issues/{id}/transition`
- `POST /api/v1/issues/{id}/comments`
- `GET /api/v1/issues`, `GET /api/v1/issues/{id}`
- `POST /api/v1/issues/{id}/archive`, `POST /api/v1/issues/{id}/unarchive`

2. Team/Auth
- `POST /api/v1/team/onboard`
- `POST /api/v1/team/role-change`
- `POST /api/v1/team/offboard`
- `GET /api/v1/team`
- `POST /api/v1/auth/issue`
- `POST /api/v1/auth/revoke`
- `GET /api/v1/auth/tokens`
- `POST /api/v1/auth/bind-role`
- `GET /api/v1/auth/role-bindings`

3. Governance/Trust
- `POST /api/v1/trust/invite`
- `POST /api/v1/trust/revoke`
- `GET /api/v1/trust`
- `POST /api/v1/governance/reconfigure`
- `POST /api/v1/governance/node-role`
- `GET /api/v1/governance`

4. Audit/Security
- `GET /api/v1/audit`
- `GET /api/v1/audit/export`
- `POST /api/v1/audit/verify-integrity`

## Attachments Model
Каждое вложение разделяется на:
1. Файл в object storage (бинарные данные).
2. Метаданные в event log:
- `attachment_id`
- `issue_id`
- `uploader_user_id`
- `storage_key`
- `filename`
- `content_type`
- `size_bytes`
- `checksum_sha256`
- `created_at`
- `deleted_at` (soft delete)

## Storage Decision
Рекомендация:
1. Хранить файлы в S3-compatible storage (MinIO/S3).
2. В event-store хранить только ссылки/метаданные и контрольные суммы.
3. Доступ к файлам через short-lived pre-signed URLs.

Почему так:
- не раздуваем `events.jsonl`;
- не ломаем репликацию событий;
- проще контролировать доступ и срок жизни ссылок;
- проще масштабировать storage независимо от compute nodes.

## Attachments API
1. Upload flow
- `POST /api/v1/issues/{id}/attachments/initiate` -> выдаёт upload URL + `attachment_id`
- клиент загружает файл напрямую в object storage
- `POST /api/v1/issues/{id}/attachments/complete` -> фиксирует событие `issue.attachment.added`

2. Read flow
- `GET /api/v1/issues/{id}/attachments` -> список метаданных
- `POST /api/v1/issues/{id}/attachments/{attachment_id}/open` -> short-lived download URL

3. Delete flow (soft)
- `POST /api/v1/issues/{id}/attachments/{attachment_id}/remove` -> `issue.attachment.removed`

## Security Requirements
- RBAC-проверка на каждом attachment endpoint.
- MIME/type + max size policy.
- Checksum verification (`sha256`) при `complete`.
- Optional malware-scan hook before marking attachment as active.
- URL TTL <= 5 минут по умолчанию.
- Все операции пишутся в аудит.

## Real-Time Requirements
- Webhook события:
  - `issue.attachment.added`
  - `issue.attachment.removed`
  - `issue.updated`
- UI/CLI live refresh должен видеть вложения без ручной синхронизации.

## Acceptance Criteria
1. Все существующие ключевые CLI операции доступны через `Master API`.
2. Attachments можно загрузить, отобразить, открыть и удалить (soft).
3. Невалидные/просроченные ссылки не дают доступ.
4. Все attachment и admin действия отражаются в audit export.
5. 3-VPS smoke проходит с минимум 20 attachment операций.
