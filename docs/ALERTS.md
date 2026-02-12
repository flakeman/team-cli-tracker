# Alert Catalog

## Purpose
Define baseline alert rules and stable alert IDs for `team-cli-tracker` operations.

## Rules

### ALT-001 SyncErrorBurst
- Severity: Sev-2
- Signal: `pull_errors`
- Condition: monotonic growth during `5m` window.
- Suggested expression:
  - `increase(pull_errors[5m]) > 0`
- Action:
  - Check peer reachability and `/sync/clock` responses.
  - Correlate with audit and recent node/network changes.

### ALT-002 SyncSuccessRatioLow
- Severity: Sev-2
- Signals: `sync_peer_pull_success`, `sync_peer_pulls`
- Condition: success ratio `< 0.95` for `15m`.
- Suggested expression:
  - `sum(increase(sync_peer_pull_success[15m])) / sum(increase(sync_peer_pulls[15m])) < 0.95`
- Action:
  - Inspect unstable peers/flapping links.
  - Check trust list and TLS/auth mismatches between nodes.

### ALT-003 SensitiveRateLimitBurst
- Severity: Sev-2
- Signal: `rate_limit_denied_sensitive`
- Condition: more than `20` denies in `5m` per node.
- Suggested expression:
  - `increase(rate_limit_denied_sensitive[5m]) > 20`
- Action:
  - Treat as abuse/scanning candidate.
  - Correlate with `/security/audit` entries (`type=rate_limit`).

### ALT-004 AuthnDenialBurst
- Severity: Sev-2
- Signal: `authn_denied_total`
- Condition: more than `50` denials in `10m` per node.
- Suggested expression:
  - `increase(authn_denied_total[10m]) > 50`
- Action:
  - Investigate token misuse/brute-force attempts.
  - Rotate/revoke impacted credentials if needed.

### ALT-005 AuthzDenialBurst
- Severity: Sev-3
- Signal: `authz_denied_total`
- Condition: more than `20` denials in `10m` per node.
- Suggested expression:
  - `increase(authz_denied_total[10m]) > 20`
- Action:
  - Review role assignments and access policy drift.
  - Validate expected automation/service accounts behavior.

## Correlation Workflow
1. Confirm alert trigger time window in UTC.
2. Pull `/metrics` snapshot from affected nodes.
3. Export `/security/audit` window for same interval.
4. Match offending paths/actors with sync errors and recent governance/team/trust changes.
5. Record incident note with alert ID(s), root cause, and remediation.
