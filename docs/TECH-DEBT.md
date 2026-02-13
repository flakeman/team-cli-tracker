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

### TD-009 Interactive command path parity with protected policy flow
- Status: Closed
- Impact: interactive `board` commands (`create/move/comment`) currently append events directly in local store and may diverge from stricter protected API/policy paths used in secured deployments.
- Current state: interactive `move` validates through protected policy path (`--interactive-policy-url` / env fallback), and interactive `create`/`comment` use protected API endpoints (`/issue/create`, `/issue/comment`) when policy URL is configured; local mode remains explicit fallback for non-protected setups.
- Exit criteria:
1. Interactive command execution supports policy-aware mode equivalent to production-protected transition path.
2. Clear mode contract in CLI/help/docs: local-append vs protected API execution.
3. Tests cover rejection/acceptance behavior parity for interactive vs non-interactive command flows.

### TD-010 Interactive mine-view identity resolution consistency
- Status: Closed
- Impact: in `--view mine`, filtering may use fallback env identity while rendered scope label can stay ambiguous, which can mislead operators about what subset is shown.
- Current state: render path persists and displays effective mine-view assignee in scope metadata (`Scope: mine(<effective-assignee>)`) including env fallback resolution; regression coverage added for resolved assignee behavior.
- Exit criteria:
1. Scope label always shows effective assignee used for filtering.
2. Interactive `view mine` behavior is deterministic and explicit when assignee is omitted.
3. Regression tests cover env-fallback and explicit-assignee scenarios.

### TD-011 Master API parity completion for issue mutation flow
- Status: Closed
- Impact: API control-plane is mostly unified, but issue mutation surface is still partially fragmented between `/issue/*` and `/raft/*` semantics.
- Current state: `/api/v1/*` namespace is available for existing handlers, including dedicated issue mutation endpoints (`/issue/create`, `/issue/transition`, `/issue/comment`) and attachment flows; compatibility path remains available for legacy routes.
- Exit criteria:
1. Stable `api/v1` endpoints cover all core issue mutations with explicit contracts (including transition path).
2. API contract tests validate parity with CLI behavior and authz outcomes.
3. Legacy path compatibility remains documented until deprecation window is defined.

### TD-012 Attachment integrity model gap for URL-only mode
- Status: Closed
- Impact: URL-mode attachments (`issue/attachment/add`) cannot provide binary integrity guarantees because file bytes are external to platform storage.
- Current state: file-mode attachments enforce `sha256` on complete and support verify/verify-all; URL-only mode is explicitly marked as non-verified (`integrity_verified=false`) and `verify` returns non-verifiable status for external URLs.
- Exit criteria:
1. URL-only mode is either restricted by policy or explicitly marked non-verified in API/UI.
2. Security/audit events distinguish verified file attachments vs URL references.
3. Documentation defines risk model and recommended production mode.

### TD-013 Periodic integrity verification execution policy
- Status: Closed
- Impact: running periodic attachment verification independently on each node can create duplicate scans and noisy audit trails at scale.
- Current state: `--attachment-verify-interval` starts scheduler only on leader policy (`preferred-leader` when set, otherwise `admin` node role), with per-node metrics exposing scheduler activity.
- Exit criteria:
1. Leader-only or single-runner scheduling policy exists for periodic verify.
2. Metrics clearly separate scheduler node from passive nodes.
3. Smoke evidence confirms no duplicate scan storms in 3-node operation.

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

### TD-014 Task lifecycle finalization (archive/delete policy)
- Status: Closed
- Impact: operators currently lack first-class archive/delete commands and retention semantics for issue lifecycle closure.
- Current state: canonical archive lifecycle is implemented in CLI/API (`issue archive|unarchive`, `/api/v1/issue/archive`, `/api/v1/issue/unarchive`) and board rendering defaults to hiding archived tasks unless `--include-archived` is explicitly set.
- Exit criteria:
1. Archive/unarchive behavior is implemented and documented.
2. Visibility/filtering rules for archived tasks are deterministic across board modes.
3. Retention/delete policy is explicitly defined (soft/hard delete boundaries). Current policy: soft archive/unarchive only; hard delete is intentionally unsupported in runtime API.

### TD-015 Technical debt register synchronization discipline
- Status: Closed
- Impact: register can drift from implemented functionality, reducing planning accuracy.
- Current state: PR template requires debt sync for architecture/security/state changes (`.github/pull_request_template.md`), and engineering process doc defines quarterly debt review and closure evidence requirements (`docs/ENGINEERING-PROCESS.md`).
- Exit criteria:
1. Debt register update is required in PR template for architecture-affecting changes.
2. Each new SDD milestone maps to debt delta (opened/closed/updated items).
3. Quarterly debt review checklist is present in docs/process.

## Repayment Plan (Suggested Sequence)
1. TD-001, TD-002, TD-003 (P0 baseline for secure production).
2. TD-004, TD-005, TD-006 (operability and resilience).
3. TD-007, TD-008 (enterprise readiness and confidence uplift).

## Working Rules
1. Every new feature touching security/state consistency must link to this register.
2. Debt item can be marked closed only when all exit criteria are met and test evidence is referenced in PR.
3. If a debt item is partially addressed, update `Current state` and keep it Open.
