# Technical Debt Register

## Purpose
Track known technical debt, prioritize repayment, and define concrete exit criteria.

## Prioritization Model
- `P0`: blocks secure/reliable production operation.
- `P1`: materially affects scale, maintainability, or resilience.
- `P2`: important improvements, but acceptable short-term risk.

## P0

### TD-001 Full consensus coverage for critical mutations
- Status: Open
- Impact: split-brain risk for governance/state-changing operations outside protected paths.
- Current state: protected transition validation exists, but not all critical writes use a single replicated state machine.
- Exit criteria:
1. All critical mutation endpoints execute through consensus-backed apply path.
2. Deterministic replay test passes across 3+ nodes after partitions/rejoins.
3. Regression tests cover conflicting concurrent writes.

### TD-002 Production-grade transport and node identity hardening
- Status: Open
- Impact: elevated risk of unauthorized node communication in misconfigured environments.
- Current state: TLS/mTLS exists, but no full lifecycle automation (issuance/rotation/revocation at scale).
- Exit criteria:
1. Node certificate issuance/renewal/revocation playbook automated.
2. Expired/revoked cert behavior covered by integration tests.
3. Hard-fail startup policy when secure mode requirements are unmet.

### TD-003 Secrets and key management maturity
- Status: Open
- Impact: operational risk for key leakage/rotation errors.
- Current state: encryption-at-rest and rotation are implemented, but no KMS integration/policy automation.
- Exit criteria:
1. KMS-backed key provider integration (or equivalent hardened external secret store).
2. Enforced rotation policy with audit evidence.
3. Recovery drill for key compromise documented and tested.

## P1

### TD-004 Real P2P discovery and peer lifecycle
- Status: Open
- Impact: manual peer management, weaker operability on dynamic networks.
- Current state: static peer list + sync APIs.
- Exit criteria:
1. Automatic peer discovery/join/leave with trust gating.
2. Peer health scoring and backoff/retry strategy.
3. Convergence tests under churn (join/leave/flapping peers).

### TD-005 Advanced abuse protection and API hardening
- Status: Open
- Impact: susceptibility to burst abuse and noisy clients.
- Current state: baseline rate limit only.
- Exit criteria:
1. Per-token/per-IP policy buckets with configurable limits.
2. Endpoint sensitivity tiers (stricter limits for auth/governance paths).
3. Security tests for brute-force/abuse scenarios.

### TD-006 Observability depth (metrics, alerts, traces)
- Status: Open
- Impact: slower incident detection and root-cause analysis.
- Current state: basic metrics and audit endpoint.
- Exit criteria:
1. SLO-driven metrics set with alert thresholds.
2. Correlated audit/event/sync diagnostics for incident timelines.
3. Runbook links to alert IDs and response procedures.

## P2

### TD-007 AuthN/AuthZ evolution for user domain
- Status: Open
- Impact: limits enterprise readiness.
- Current state: static token map and RBAC wrappers.
- Exit criteria:
1. External identity integration path (OIDC/SSO) or hardened local issuer.
2. Token/session lifecycle (expiry, refresh, revoke-at-scale).
3. Role binding management model with migration compatibility.

### TD-008 Test matrix expansion (chaos and long-running scenarios)
- Status: Open
- Impact: latent defects under real-world fault patterns.
- Current state: good unit/integration coverage on core flows, limited chaos duration tests.
- Exit criteria:
1. Partition/rejoin chaos suite with deterministic assertions.
2. Long-running sync soak test.
3. Governance reconfiguration stress tests with no divergence.

## Repayment Plan (Suggested Sequence)
1. TD-001, TD-002, TD-003 (P0 baseline for secure production).
2. TD-004, TD-005, TD-006 (operability and resilience).
3. TD-007, TD-008 (enterprise readiness and confidence uplift).

## Working Rules
1. Every new feature touching security/state consistency must link to this register.
2. Debt item can be marked closed only when all exit criteria are met and test evidence is referenced in PR.
3. If a debt item is partially addressed, update `Current state` and keep it Open.
