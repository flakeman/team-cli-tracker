# SDD-11 Plan: Production Deployment And Real User Onboarding

## Objective
Сделать процесс развёртывания на Ubuntu 24 и подключения реальных пользователей воспроизводимым и проверяемым через аудит.

## Steps
1. Зафиксировать deployment flow для 3 VPS из prebuilt binary (без Go на серверах).
2. Описать запуск `screen` и `systemd` для эксплуатационных режимов.
3. Описать onboarding реальных пользователей через issuer/policy (roles + tokens).
4. Добавить сценарий валидации authz (allow/deny matrix).
5. Зафиксировать аудит-чеклист (`export` + `verify-integrity`) как обязательный этап.

## Exit
- Оператор может по документации развернуть кластер и подключить реальных пользователей без неявных шагов.
