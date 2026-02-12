# Security Runbook And Incident Response

## Purpose
This runbook defines baseline operational security checks and incident response steps for `team-cli-tracker` clusters.

## Scope
- Node-to-node transport security (TLS/mTLS).
- User/API authorization controls.
- Trust list and team lifecycle controls.
- Event chain integrity and encrypted payload handling.
- Abuse protection and auditability.

## Daily Security Checks
1. Verify node health and security endpoint:
   - `teamcli security status --node <url>`
   - `curl -s http://127.0.0.1:4101/metrics`
2. Check recent denied requests and auth failures:
   - `teamcli security audit --since 24h --type auth.failed`
3. Check trust list consistency:
   - `teamcli trust list`
4. Check team membership and revoked users:
   - `teamcli team list-members --project <id>`
5. Verify no integrity errors:
   - `teamcli events verify-chain --project <id>`

## Baseline Metric Thresholds
- `pull_errors`: page if continuously increasing for `>5m`.
- `sync_peer_pull_success / sync_peer_pulls`: investigate if below `0.95` for `>15m`.
- `rate_limit_denied_sensitive`: investigate burst if `>20` per `5m` per node.
- `authn_denied_total`: investigate if growth is `>50` per `10m` per node.
- `authz_denied_total`: investigate if growth is `>20` per `10m` per node.
- Alert IDs and rule details: `docs/ALERTS.md` (`ALT-001`..`ALT-005`).

## Key Operations
1. Rotate encryption key:
   - `go run ./cmd/node storage rotate-key --data-dir ./data --enforce-due --max-age 720h`
2. Validate backward-compatible decryption:
   - `go run ./cmd/node storage recovery-drill --data-dir ./data`
3. Record change in audit:
   - `storage.key.enable`, `storage.key.rotate`, `storage.key.policy_check`, `storage.key.recovery_drill`
4. Check policy due state:
   - `go run ./cmd/node storage key-policy-check --data-dir ./data --max-age 720h`
5. External secret store mode (equivalent hardened provider):
   - `EVENT_KEY_PROVIDER=file`
   - `EVENT_KEY_PROVIDER_FILE=/etc/team-cli-tracker/external-keys.json`

## Node Join Hardening
1. Generate short-lived invite token:
   - `teamcli trust invite --ttl 15m`
2. Add node to allowlist only after ownership verification.
3. Require mTLS cert signed by trusted CA.
4. Confirm node appears as expected role (`voting` or `non-voting`).

## Offboarding Security
1. Disable member and revoke tokens:
   - `teamcli team offboard --member <id> --project <id> --policy lead`
2. Verify reassignment events were written.
3. Confirm member can no longer run issue commands.
4. Archive offboarding reason in audit log.

## Governance Reconfiguration Safety
1. Use odd-sized voting set only.
2. Reconfigure with explicit node list:
   - `teamcli governance reconfigure --voting node-a,node-c,node-d`
3. Confirm returned quorum and node roles.
4. Watch sync and transition validation for at least one full cycle.

## Incident Response Checklist
1. Detect:
   - Alert from failed mTLS, auth bursts, integrity mismatch, or unexpected role change.
   - Record triggered alert ID(s) from `docs/ALERTS.md`.
2. Contain:
   - Revoke suspicious node/member.
   - Block invite issuance temporarily.
   - Tighten rate limits.
3. Eradicate:
   - Rotate affected keys/tokens.
   - Remove unauthorized trust/team entries.
4. Recover:
   - Re-sync nodes.
   - Verify chain integrity and board correctness.
   - Re-enable normal rate limits.
5. Post-incident:
   - Export audit window.
   - Document root cause and policy updates.
   - Add regression test if gap was code-related.

## Severity Guide
- Sev-1: Unauthorized state mutation or cross-node trust compromise.
- Sev-2: Repeated auth bypass attempts, partial service denial, or invalid role escalation attempt.
- Sev-3: Single-user token abuse, noisy scans, minor policy misconfiguration.

## Evidence To Capture
- Time window (UTC).
- Affected project and issue IDs.
- Node IDs and member IDs involved.
- Relevant audit event IDs.
- Commands/actions taken during containment and recovery.
