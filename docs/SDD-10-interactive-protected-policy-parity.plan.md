# SDD-10 Plan

## Phase 1 - Contract
- Define interactive protected-mode inputs and fallback rules.
- Document behavior matrix (policy URL set vs unset).

## Phase 2 - Transition Parity
- Wire interactive `move` through protected transition validation.
- Preserve local append behavior only after successful validation.

## Phase 3 - Verification
- Add tests for policy allow/deny outcomes.
- Update technical debt record (`TD-009`) with implemented scope.
