# SDD-11: Production Deployment And Real User Onboarding

## Status
- Completed
- Date: 2026-02-13
- Revision: 1

## Context
Текущие smoke-прогоны на 3 VPS доказали сетевую и функциональную работоспособность кластера, но нужен формальный, повторяемый процесс:
- production-like развертывания на Ubuntu 24;
- подключения реальных пользователей через issuer/policy;
- верификации реальных действий через аудит.

## Problem
Без стандартизованного процесса операторы путают:
- "сколько VPS" и "сколько пользователей";
- тестовых акторов (`node-id`/симуляция) и реальных пользователей (token-based identities);
- одноразовый ручной запуск и поддерживаемый эксплуатационный контур.

## Goals
1. Зафиксировать эталон развертывания 3-node кластера на Ubuntu 24 без установки Go на серверы (через prebuilt binary).
2. Зафиксировать процесс онбординга реальных пользователей (role + token + client session).
3. Зафиксировать проверку прав и аудит-доказательство реальных действий.
4. Дать два режима эксплуатации:
- быстрый (`screen`) для оперативной валидации,
- устойчивый (`systemd`) для постоянного сервиса.

## Non-Goals
- Реализация внешнего IdP/SSO провайдера.
- Полная автоматизация CI/CD деплоя.
- Multi-tenant изоляция по нескольким организациям.

## Functional Requirements
1. Deployment
- Возможность запускать узлы из бинарника `node` без `go` на VPS.
- Поддержка конфигурирования `--project-id`, `--listen`, `--data-dir`, `--secure-mode-required`.
- Проверяемое межузловое соединение (3 VPS quorum).

2. Real User Connectivity
- Каждый пользователь использует уникальный `user_id`.
- Аутентификация выполняется через bearer token issuer.
- Авторизация выполняется через role/policy (`admin`, `lead`, `dev`, `qa`, `viewer`).
- Пользователь подключается к доске отдельной клиентской сессией (`watch`/`interactive`).

3. Audit Evidence
- Все значимые действия (`create`, `move`, `comment`, team/trust/security operations) видны в audit export.
- Поддержка фильтрации экспорта: `all`, `period`, `users`.
- Наличие integrity проверки (`audit verify-integrity`).

## Operational Requirements
- Поддержка запуска в `screen` для отладки/демо.
- Поддержка запуска через `systemd` для прод-эксплуатации.
- Прозрачный runbook для восстановления после рестарта VPS.
- Возможность быстро отличить simulated actors от real users в отчете аудита.

## Acceptance Criteria
1. На 3 VPS под Ubuntu 24 поднят кластер и доступен board API.
2. Минимум 5 реальных ролей/пользователей подключены через токены и выполняют свои допустимые операции.
3. Negative authz проверки проходят (например, `viewer` не может `move`).
4. Аудит-выгрузка подтверждает фактические `user_id`, роли и временные метки.
5. Runbook позволяет повторить разворачивание "с нуля" без Go на сервере.

## Artifacts
- `docs/DEPLOYMENT-UBUNTU24-3VPS-REAL-USERS.md`
- `docs/SECURITY-RUNBOOK.md` (используется совместно)
- `docs/SMOKE-EXTENDED-3VPS-2026-02-13.md` (фактическое smoke-evidence)

## Risks
- Неправильная выдача ролей в issuer может маскировать нарушения RBAC.
- Хранение токенов в shell history/скриптах без секрет-менеджмента.
- Расхождения часовых поясов на узлах при разборе аудита.

## Mitigations
- Минимально необходимые роли, принцип least privilege.
- Короткоживущие токены + ротация.
- UTC на всех узлах.
- Обязательная проверка `audit verify-integrity` после критичных прогонов.

## Follow-up
1. Автоматизировать onboarding/offboarding через API workflow.
2. Добавить образцовые `systemd` unit templates в `deploy/systemd`.
3. Добавить dedicated smoke profile "real users only (no simulated actors)".
