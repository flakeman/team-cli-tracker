# SDD-03 Security And Team Lifecycle

## Context
SDD-02 закрыл MVP по подписанным событиям, sync и защищенным переходам. Для production-ready эксплуатации нужны полноценные security controls и управляемый жизненный цикл участников проекта.

## Goal
Добавить системный контур безопасности и командного управления: authN/authZ, защищенный transport, контроль членства, корректный offboarding и автоматическое переназначение задач.

## Scope
1. Security hardening (transport, identity, secrets, storage).
2. Team lifecycle: onboarding/offboarding/role change.
3. Assignment continuity: правила переназначения при уходе участников.
4. Voting/non-voting node governance для динамического состава команды.

## Out Of Scope
- Полная интеграция с внешним IAM/SSO провайдером в первой итерации.
- Mobile/web UI администратора (CLI/API only).

## Functional Requirements
1. Transport security:
- TLS для всех node-to-node и client-to-node API.
- mTLS для межнодового трафика (trusted CA / pinned certs).

2. Authentication and authorization:
- user token auth для CLI-команд.
- role-based authorization (`admin`, `lead`, `dev`, `qa`, `viewer`) на уровне policy.
- запрет операций при revoked/expired token.

3. Node trust:
- allowlist доверенных нод в проекте.
- join/invite flow для новых нод с ограниченным TTL invite.
- revoke flow для исключения нод.

4. Event/data security:
- encryption-at-rest для чувствительных payload.
- key rotation policy (проектные ключи, node keys).
- проверка целостности event chain.

5. Team lifecycle:
- onboarding: создать участника, назначить роль, добавить в проект, выдать доступ.
- role change: изменить роль без остановки проекта.
- offboarding: деактивировать доступ и отозвать токены.

6. Assignment continuity:
- policy reassign при уходе исполнителя:
  - primary: team lead
  - fallback: component duty
  - fallback2: `unassigned` + alert
- фиксировать в audit trail, кто и почему переназначен.

7. Node governance:
- разделение voting и non-voting узлов.
- безопасная смена состава voting set (rolling reconfiguration).
- правила для even team size: witness node / odd quorum enforcement.

8. Operational security:
- rate limit и basic abuse protection для API.
- security audit log (auth failures, policy denials, key operations).
- health/security endpoints для наблюдаемости.

## Non-Functional Requirements
- Security policy changes apply without full cluster restart.
- Critical auth decision latency p95 < 250ms.
- No unauthorized transition accepted under concurrent load tests.
- Key rotation must not break ongoing sync.

## Acceptance
1. Node-to-node requests without valid mTLS cert are rejected.
2. Revoked user token can no longer execute `issue` commands.
3. Offboarded user tasks are reassigned according to policy and visible in audit.
4. Voting set change completes without data divergence.
5. Security audit events are queryable and include actor/time/reason.

