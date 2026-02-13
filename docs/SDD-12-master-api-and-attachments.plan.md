# SDD-12 Plan: Master API And File Attachments

## Phase 1 - API Unification
1. Ввести namespace `/api/v1/*` поверх существующих handler'ов.
2. Закрыть API parity с CLI (issues/team/trust/governance/auth/audit).
3. Добавить contract tests на все endpoints.

## Phase 2 - Attachments MVP
1. Добавить attachment metadata model и события (`added/removed`).
2. Реализовать S3-compatible storage adapter.
3. Реализовать initiate/complete/open/remove API.
4. Добавить RBAC, size/type/checksum policies.

## Phase 3 - Ops And Integrations
1. Добавить webhook delivery для attachment событий.
2. Добавить 3-VPS smoke сценарий с файлами.
3. Обновить runbook (backup, retention, recovery).

## Exit
- Master API становится primary control plane.
- Attachments работают end-to-end и покрыты аудитом.
