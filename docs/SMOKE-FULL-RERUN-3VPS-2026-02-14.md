# Smoke Re-Run Report: Full Matrix + Multi-User + Files (2026-02-14 UTC)

## Goal
Repeat full end-to-end validation from scratch:
- commands/API coverage,
- multiple users/roles,
- file attachment lifecycle,
- webhook lifecycle,
- board render modes,
- cross-node observer checks.

## Execution Topology
- Primary execution node: `srv1.example.internal (SSH port 22)` (`srv1-22221`) (fresh binary + fresh data-dir)
- Observer nodes:
  - `srv2.example.internal (SSH port 22)` (`srv2-22222`)
  - `srv3.example.internal (SSH port 22)` (`srv3-22223`)
- API test port on primary: `:4111`

## Matrix Result (Primary Node)
All checks passed with expected status codes.

- Issues:
  - create (`15/15`): `201`
  - transition (`8/8`): `201`
  - comment (`8/8`): `201`
  - archive/unarchive: `201/201`
- RBAC negative:
  - `viewer create`: `403` (expected)
  - `viewer comment`: `403` (expected)
- Attachments:
  - `initiate`: `200`
  - `upload`: `201`
  - `complete`: `201`
  - `open`: `200`
  - `verify`: `200`
  - `verify-all`: `200`
- Webhooks:
  - `create`: `201`
  - `list`: `200`
  - `test`: `200`
  - `revoke`: `200`
- Team/Auth/Audit:
  - `team/onboard`: `201`
  - `team/list`: `200`
  - `auth/list`: `200`
  - `security/audit/export`: `200`
- Board CLI:
  - `board --once`: `0`
  - `board --once --include-archived`: `0`
  - `board --counts-only --once`: `0`

Conclusion (primary): `PASS`.

## Observer Nodes Check
From `srv2-22222` and `srv3-22223` to `http://srv1.example.internal:4111`:
- `curl /healthz` -> `000` + `connection refused`
- `board --peers http://srv1.example.internal:4111` -> sync clock error + empty board

Conclusion (cross-node): `BLOCKED` by inter-node network reachability on `4111/tcp`.

## Required Infra Fix
Open `4111/tcp` between all three nodes (both directions), then re-run observer section.

