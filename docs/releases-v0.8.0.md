# Release Notes

## Version
`v0.8.0`

## Date
`2026-02-12`

## Summary
`v0.8.0` closes the initial technical debt track and upgrades the project to an operator-ready baseline: stronger security controls, broader consensus coverage, improved sync resilience, and a production-style board view in CLI.

## Highlights
1. Consensus-protected mutations extended across team, trust, governance, and transition validation paths.
2. Security maturity improvements: auth issuer lifecycle, sensitive endpoint rate limits, secure startup policy, and CRL-aware mTLS checks.
3. Storage/key operations expanded with rotation policy checks, recovery drill flow, and external key provider support.
4. `board` plain output now renders as a structured table and can run in active refresh mode.

## Technical Details
- Auth/Security:
  - Local token issuer with issue/revoke/list commands and role binding migration compatibility.
  - Sensitive endpoint limiter and stable client IP keying.
  - Secure mode gate and certificate revocation list integration.
- Sync/Consensus:
  - Additional consensus gates for critical mutations.
  - Discovery/backoff metrics and retry behavior under temporary peer loss.
- Storage/Key Management:
  - Rotation policy checks and startup enforcement.
  - Recovery drill command and audit coverage.
  - External key provider file mode support.
- CLI/UX:
  - Table-style plain board renderer with column limits/counts.
  - Live board refresh loop with optional peer sync inputs.

## Breaking Or Behavioral Changes
- `board` command behavior changed: default is live-refresh mode; use `--once` for previous one-shot output behavior.

## Validation
- Unit/integration tests:
  - `go test ./...` => pass (local run on release prep).
- 3-VPS smoke:
  - `docs/SMOKE-MAIN-3VPS-2026-02-12.md` => pass with one noted audit-event follow-up.

## Upgrade Notes
1. Required env vars:
  - No new mandatory env vars for default mode.
2. Recommended rollout order:
  - Upgrade one node, verify `/healthz` and `/metrics`, then roll through remaining nodes.
