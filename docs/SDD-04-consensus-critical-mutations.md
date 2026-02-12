# SDD-04 Consensus For Critical Mutations

## Context
В текущем состоянии часть критичных изменений уже проходит через защищенную quorum-валидацию (workflow transitions), но не все state-changing операции используют единый consensus-backed apply path.

## Goal
Убрать риск расхождения критичного состояния, расширив consensus gate и далее переведя критичные мутации на единый подтверждаемый контур.

## Scope
1. Расширение quorum-валидации на governance/team/trust мутации.
2. Единый mutation envelope для критичных операций.
3. Детализированные интеграционные тесты расхождений и rejoin-сценариев.

## Out Of Scope
- Полная замена текущей sync-модели на новый движок в этой итерации.
- UI-уровень подтверждений.

## Functional Requirements
1. Критичные операции выполняются только после quorum grant.
2. Валидация должна включать проверку целостности payload и project scope.
3. Результат apply фиксируется в audit trail с granted/needed quorum.
4. При недостижении quorum операция отклоняется без частичного apply.

## Non-Functional Requirements
- p95 latency для validate path < 400ms в 3-node локальном стенде.
- Нет divergence в тестах partition/rejoin после серии критичных мутаций.

## Acceptance
1. Governance reconfigure проходит только при quorum и отвергается при invalid/even voting set.
2. Team offboard (критичный path) подтверждается quorum-gate в следующей итерации.
3. Интеграционные тесты подтверждают一致ность состояния после rejoin.
