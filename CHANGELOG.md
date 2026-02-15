# Changelog

All notable changes are documented here.

## Unreleased

### Added
- `smoke.ps1` for end-to-end operator smoke from PowerShell:
  - issue create/move/comment sequence
  - role-deny checks
  - attachment initiate/upload/complete/verify flow
  - 2-3s transition cadence for live board observation.
- Isolated deployment artifacts for storage/networking:
  - WireGuard install/env templates
  - distributed MinIO install/systemd/bootstrap/healthcheck scripts
  - `docs/DEPLOYMENT-ISOLATED-WG-HA.md` with single-entrypoint HA modes.

### Changed
- Deploy/runtime validation on 3-node VPS cluster moved to `systemd` service (`team-cli-rerun.service`) with:
  - `secure-mode-required=true`
  - unified node IDs (`srv1/srv2/srv3`)
  - peer mesh over `:4101`.

### Verified
- Cross-node consistency smoke on `srv1/srv2/srv3`:
  - 40-issue bulk scenario
  - churn check (`srv2` restart + re-sync)
  - attachment replication and integrity verification across nodes.

## v0.8.0 - 2026-02-12

### Added
- Full consensus coverage for critical mutation endpoints (`team`, `trust`, `governance`, transition validation).
- Deterministic replay and conflict convergence suites for partition/rejoin scenarios.
- Peer lifecycle improvements:
  - trust-gated discovery (`/sync/peers`)
  - TTL prune and revoke-aware peer cleanup
  - backoff/retry state and metrics.
- Security and abuse hardening:
  - sensitive endpoint rate-limit tier
  - stable client IP keying
  - extended observability metrics and alert catalog.
- Transport hardening:
  - secure startup policy enforcement
  - CRL-backed mTLS revocation checks
  - PKI helper script (`deploy/pki-manage.sh`).
- Key management maturity:
  - key rotation policy checks and startup enforcement
  - recovery drill command and audit evidence
  - external key provider support (`EVENT_KEY_PROVIDER=file`).
- Auth lifecycle domain:
  - local issuer (`internal/auth`)
  - token issue/revoke/list
  - role bindings with static-token migration compatibility.

### Changed
- `board --format plain` now renders as tabular Kanban view.
- `board` defaults to live mode with periodic refresh and optional peer sync.

### Docs
- Added `docs/ALERTS.md`.
- Added `docs/SDD-05-board-smoke-and-release.*`.
- Updated `docs/TECH-DEBT.md` with closed status for all tracked items.
