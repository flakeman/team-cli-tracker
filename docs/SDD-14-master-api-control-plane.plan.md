# SDD-14 Plan

## Phase 1: Bootstrap
1. Add `master/health` and `master/capabilities` endpoints.
2. Protect endpoints by `admin|lead`.
3. Add tests for auth + response contract.

## Phase 2: Namespace parity
1. Introduce `/api/v1/master/*` aliases for existing domains.
2. Add audit event type prefix `master.*`.
3. Preserve backward compatibility with current paths.

## Phase 3: Cluster control
1. Add cluster health summary endpoint.
2. Add node lifecycle/reconfigure orchestrator endpoints.
3. Add safety guards (quorum/rate-limit/idempotency).

## Verification
1. `go test ./...` green.
2. 3-node smoke: bootstrap and selected master mutations.
3. Release checklist updated with master control-plane gate.
