# Smoke Report: Full Command Matrix + Multi-User + Files (2026-02-13)

## Scope
- Full command/API matrix on fresh test data.
- Multi-user role matrix (`admin`, `lead`, `dev`, `qa`, `viewer`).
- File creation and attachment lifecycle (`initiate/upload/complete/open/verify/verify-all`).
- Webhook lifecycle (`create/list/test/revoke`).
- Board rendering checks (`once`, `counts`, `include-archived`).

## Environment
- Main execution node: `srv1.abuztech.ru:22` (`srv1-22221`)
- Observer nodes:
  - `srv2.abuztech.ru:22` (`srv2-22222`)
  - `srv3.abuztech.ru:22` (`srv3-22223`)
- Test API port: `:4111`
- Binary: freshly built from current `main` (`node-linux-fulltest` uploaded to `srv1.abuztech.ru`).

## Main Matrix Result (srv1-22221)
- Created issues: `12/12` (`201`)
- Transitions: `6/6` (`201`)
- Comments: `6/6` (`201`)
- Archive/unarchive: `201/201`
- RBAC negative checks (viewer create/comment): `403/403` (expected)
- Attachment flow:
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
- Board CLI checks:
  - `board --once`: `0`
  - `board --once --include-archived`: `0`
  - `board --counts-only --once`: `0`

Conclusion for `srv1-22221`: PASS.

## 3-VPS Observer Check (srv2-22222 / srv3-22223)
- From `srv2-22222` and `srv3-22223`:
  - direct `curl` to `http://srv1.abuztech.ru:4111/healthz` -> connection refused (`000`)
  - board peer sync to `http://srv1.abuztech.ru:4111` -> sync clock error / empty board

Conclusion for cross-node e2e: BLOCKED by network reachability (port `4111` inaccessible from other VPS).

## Blocking Issue
- `4111/tcp` is not reachable from `srv2-22222` and `srv3-22223` to `srv1-22221`.
- Until security-group/firewall routing is fixed, full 3-VPS live sync validation cannot be marked PASS.

## Next Action
1. Open/allow `4111/tcp` between all three VPS.
2. Re-run the same matrix with peer sync enabled from `srv2-22222` and `srv3-22223`.
3. Attach updated evidence and mark cross-node section PASS.
