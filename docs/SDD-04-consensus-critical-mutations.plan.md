# SDD-04 Plan

## Phase 1 - Governance quorum gate
- Add Raft vote/validate endpoints for governance reconfigure.
- Enforce quorum validation before applying voting set changes.
- Add tests for success and invalid voting set rejection.

## Phase 2 - Team lifecycle critical operations
- Add consensus gate for `team offboard` (and reassignment side-effects).
- Ensure deterministic apply order for offboard + reassignment events.

## Phase 3 - Trust and policy mutations
- Move trust invite/revoke critical paths behind consensus validation.
- Add audit fields for quorum metadata.

## Phase 4 - Divergence testing
- Add partition/rejoin suite for combined governance/team mutations.
- Validate no divergence with rolling membership changes.
