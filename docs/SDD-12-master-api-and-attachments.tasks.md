# SDD-12 Tasks

- [x] Зафиксировать архитектуру Master API и API parity цель.
- [x] Зафиксировать решение по вложениям: object storage + metadata in events.
- [x] Описать attachment upload/open/remove flow.
- [x] Определить security policy для вложений (RBAC, TTL, checksum, limits).
- [x] Реализовать `/api/v1/*` слой для всех ключевых команд.
- [x] Реализовать attachment events (MVP links) и API add/list/open/remove.
- [x] Реализовать storage adapter для бинарных файлов (MVP local + S3-compatible backend).
- [x] Реализовать presigned upload/download lifecycle (MVP signed tokens).
- [x] Добавить checksum validation (`sha256`) в attachment complete flow.
- [ ] Добавить webhook события для вложений.
- [ ] Добавить e2e smoke: 3 VPS + attachments + audit evidence.
