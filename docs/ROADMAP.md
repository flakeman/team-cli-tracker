# Roadmap

## Current Status (as of 2026-02-12)

### Done
- Node process skeleton and local event store.
- Signed event pipeline and deterministic replay/convergence tests.
- Peer sync with trust-gated discovery, backoff, churn handling.
- Consensus gate for critical mutations (team/trust/governance/transition paths).
- RBAC/auth hardening (issuer, token lifecycle, role bindings, authn/authz guards).
- Security controls (rate-limit tiers, secure mode policy, mTLS + CRL checks).
- Storage hardening (encryption, rotation policy checks, recovery drill, integrity verify).
- Board UX:
  - canonical table rendering,
  - active live refresh mode,
  - 3-VPS smoke evidence for active updates.
- End-to-end audit baseline:
  - CLI/API export (`all`, `period`, `users`),
  - pagination cursor,
  - hash-chain integrity verification,
  - redaction on write.

### In Progress / Partial
- Full API surface unification for all CLI capabilities.
- Outbound integration layer for external systems.

## Milestone M1 (Completed)
- Node process skeleton.
- Local event store.
- Signed events.
- P2P peer discovery + sync.
- CLI board read.

## Milestone M2 (Completed)
- Raft policy service.
- RBAC enforcement.
- Workflow configuration.
- Transition guards.

## Milestone M3 (Mostly Completed)
- CRDT merge behavior in replicated projection path.
- Ops hardening (security/observability/smoke/release baseline).
- Remaining from original M3:
  - generalized snapshot/restore workflow polish,
  - notification delivery layer.

## Milestone M4 (Next) - Integrations And Full API

### Why API + Webhooks (not one of them)
- `API` is the primary control plane for full functionality (CRUD, transitions, team/trust/governance/auth/audit/export).
- `Webhooks` are the event delivery plane for outbound push to external systems.
- For Jira integration this combination is best:
  - API for deterministic pull/sync/reconciliation and backfill.
  - Webhooks for near-real-time updates without polling pressure.

### Scope
- Full public API parity with CLI commands.
- Outbound webhooks for all key domain events:
  - issue created/updated/transitioned/commented,
  - team onboard/role-change/offboard,
  - trust invite/revoke,
  - governance changes,
  - auth/audit significant events.
- Jira integration:
  - mapping `OPS-*` <-> Jira Issue keys,
  - two-way status/assignee/comment sync policy,
  - idempotency keys and replay-safe delivery,
  - retry/backoff + dead-letter handling.

### Exit Criteria
1. Every major CLI function has stable API endpoint parity.
2. Webhook subscription management exists (create/list/revoke/test).
3. Jira connector supports:
   - initial import/backfill,
   - incremental sync,
   - conflict policy with audit evidence.
4. Integration smoke on 3 VPS includes Jira sync + webhook delivery checks.
