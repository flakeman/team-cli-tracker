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
2. Check recent denied requests and auth failures:
   - `teamcli security audit --since 24h --type auth.failed`
3. Check trust list consistency:
   - `teamcli trust list`
4. Check team membership and revoked users:
   - `teamcli team list-members --project <id>`
5. Verify no integrity errors:
   - `teamcli events verify-chain --project <id>`

## Key Operations
1. Rotate encryption key:
   - `teamcli keys rotate --project <id>`
2. Validate backward-compatible decryption:
   - Read historical issue/comment payloads after rotation.
3. Record change in audit:
   - Actor, reason, timestamp, affected project.

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
