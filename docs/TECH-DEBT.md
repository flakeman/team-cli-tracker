# Technical Debt Register

## Purpose
Track known technical debt, prioritize repayment, and define concrete exit criteria.

## Prioritization Model
- `P0`: blocks secure/reliable production operation.
- `P1`: materially affects scale, maintainability, or resilience.
- `P2`: important improvements, but acceptable short-term risk.

## P0

### TD-001 Full consensus coverage for critical mutations
- Status: Closed
- Impact: split-brain risk for governance/state-changing operations outside protected paths.
- Current state: critical mutation endpoints use quorum-backed apply paths for transition validation, governance reconfigure, governance node-role, team onboard/role-change/offboard, and trust invite/revoke. Deterministic replay and conflict convergence coverage is in place (`TestProjectBoardFromEventsDeterministicReplayOrder`, `TestThreeNodeConflictingTransitionsDeterministicAfterRejoin`) alongside churn/rejoin suites.
- Exit criteria:
1. All critical mutation endpoints execute through consensus-backed apply path.
2. Deterministic replay test passes across 3+ nodes after partitions/rejoins.
3. Regression tests cover conflicting concurrent writes.

### TD-002 Production-grade transport and node identity hardening
- Status: Closed
- Impact: elevated risk of unauthorized node communication in misconfigured environments.
- Current state: startup hard-fails on insecure/partial peer transport config (`SECURE_MODE_REQUIRED`); PKI automation exists for CA init + node cert issue/revoke (`deploy/pki-manage.sh`); mTLS runtime coverage includes expired and revoked client certificate rejection (`TestMTLSRejectsExpiredClientCertRuntime`, `TestMTLSRejectsRevokedClientCertRuntime`) with CRL support (`TLS_CRL_FILE`).
- Exit criteria:
1. Node certificate issuance/renewal/revocation playbook automated.
2. Expired/revoked cert behavior covered by integration tests.
3. Hard-fail startup policy when secure mode requirements are unmet.

### TD-003 Secrets and key management maturity
- Status: Closed
- Impact: operational risk for key leakage/rotation errors.
- Current state: encryption-at-rest and key rotation are implemented; rotation policy is enforceable (`ENFORCE_KEY_ROTATION_POLICY`, `KEY_ROTATION_MAX_AGE`) with CLI policy checks and audit evidence (`storage.key.*` events); recovery drill command/test are in place; external hardened key provider integration is available (`EVENT_KEY_PROVIDER=file`, `EVENT_KEY_PROVIDER_FILE`) with round-trip coverage (`TestExternalFileKeyProviderRoundTrip`).
- Exit criteria:
1. KMS-backed key provider integration (or equivalent hardened external secret store).
2. Enforced rotation policy with audit evidence.
3. Recovery drill for key compromise documented and tested.

## P1

### TD-004 Real P2P discovery and peer lifecycle
- Status: Closed
- Impact: manual peer management, weaker operability on dynamic networks.
- Current state: sync layer does trust-gated peer discovery (`/sync/peers`), dynamic join/prune with TTL/revocation checks, peer health backoff/retry metrics, and deterministic convergence coverage under churn/flapping (`TestThreeNodeConvergeUnderChurnFlapping`).
- Exit criteria:
1. Automatic peer discovery/join/leave with trust gating.
2. Peer health scoring and backoff/retry strategy.
3. Convergence tests under churn (join/leave/flapping peers).

### TD-005 Advanced abuse protection and API hardening
- Status: Closed
- Impact: susceptibility to burst abuse and noisy clients.
- Current state: per-token/per-IP buckets, endpoint sensitivity tiers (`default`/`sensitive`), and abuse-oriented tests are in place.
- Exit criteria:
1. Per-token/per-IP policy buckets with configurable limits.
2. Endpoint sensitivity tiers (stricter limits for auth/governance paths).
3. Security tests for brute-force/abuse scenarios.

### TD-006 Observability depth (metrics, alerts, traces)
- Status: Closed
- Impact: slower incident detection and root-cause analysis.
- Current state: metrics include sync health (`sync_peer_pulls`, `sync_peer_pull_success`) and security denials (`rate_limit_denied_*`, `authn_denied_total`, `authz_denied_total`); runbook baseline thresholds are mapped to stable alert IDs in `docs/ALERTS.md` and incident response references those IDs.
- Exit criteria:
1. SLO-driven metrics set with alert thresholds.
2. Correlated audit/event/sync diagnostics for incident timelines.
3. Runbook links to alert IDs and response procedures.

## P2

### TD-007 AuthN/AuthZ evolution for user domain
- Status: Closed
- Impact: limits enterprise readiness.
- Current state: hardened local issuer is available (`internal/auth` + `/auth/*` + `node auth ...`) with persisted tokens, expiry/revocation lifecycle, role bindings, and migration compatibility via static token seeding (`AUTH_TOKENS_JSON` -> auth store). Coverage includes lifecycle and compatibility tests (`TestAuthIssueAndRevokeLifecycle`, `TestSeedStaticMigrationCompatibility`).
- Exit criteria:
1. External identity integration path (OIDC/SSO) or hardened local issuer.
2. Token/session lifecycle (expiry, refresh, revoke-at-scale).
3. Role binding management model with migration compatibility.

### TD-008 Test matrix expansion (chaos and long-running scenarios)
- Status: Closed
- Impact: latent defects under real-world fault patterns.
- Current state: deterministic chaos/soak/governance-stress coverage is in place (`TestChaosPartitionRejoinDeterministicConvergence`, `TestLongRunningSyncSoakDeterministic`, `TestGovernanceReconfigureStressNoDivergence`) in addition to existing partition/rejoin suites.
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
