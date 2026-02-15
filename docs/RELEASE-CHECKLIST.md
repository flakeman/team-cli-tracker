# Release Checklist (Go/No-Go)

Date: 2026-02-15  
Release: pre-v0.x  
Owner: vova

## 1) Network
- [x] Inter-node `4101/tcp` is reachable both directions: `srv1 <-> srv2 <-> srv3`.
- [x] Public/consumer entrypoint is defined (`board DNS` or local-only mode). (isolated mode: `board.internal -> 10.20.0.10` via VIP)
- [x] Single entrypoint failover is configured (LB or VRRP VIP or DNS health-check).
- [x] SSH keys are configured; password SSH is disabled or restricted. (inter-node key auth enabled; external password auth retained as break-glass access)

## 2) Runtime
- [x] `team-cli-tracker` runs under `systemd` on all 3 nodes. (`team-cli-rerun.service`)
- [x] Service restart policy is enabled (`Restart=always`).
- [x] Health checks return `ok` on all nodes:
  - [x] `srv1`: `/healthz`
  - [x] `srv2`: `/healthz`
  - [x] `srv3`: `/healthz`
- [x] Node config is consistent (`node-id`, `public-url`, `peers`, `project-id`).

## 3) Security
- [x] `secure-mode-required=true` in prod.
- [x] Auth is enabled and role policy validated (`admin/lead/dev/qa/viewer`).
- [x] Token rotation procedure exists and was tested. (local drill: enable-encryption -> rotate-key -> recovery-drill passed on 2026-02-15)
- [x] TLS/mTLS decision is documented and applied (or private-trust boundary is approved). (approved private-trust boundary: WireGuard-only contour, no public exposure)

## 4) Data and Recovery
- [x] Backup policy for `data-dir` exists (schedule + retention). (documented via backup/restore runbook + scripts)
- [x] Restore drill was executed successfully. (srv1 drill with archive restore and board projection check on 2026-02-15)
- [x] Audit export works (`all`, `period`, `users`).
- [x] Audit integrity verification passes.

## 5) Attachments
- [x] Attachment backend is selected (`local` or `s3`) and documented. (prod baseline: `s3`, dev fallback: `local`)
- [x] Distributed MinIO (`x3`) is deployed and healthy.
- [x] `s3.internal` endpoint is stable via selected HA profile. (selected profile: keepalived VIP in isolated contour)
- [x] `initiate -> upload -> complete -> verify` flow passes.
- [x] `verify-all` passes for current project.
- [x] Storage capacity/cleanup policy exists. (retention/cleanup policy documented in operations runbook)

## 6) Functional E2E
- [x] 30-40 issues scenario executed. (40 issues bulk run completed on 2026-02-15)
- [x] All core commands tested: create, move, comment, archive/unarchive, board modes.
- [x] Master control-plane smoke passed (`/master/health`, `/master/cluster/health`, mutating idempotency). (3-node smoke on 2026-02-15)
- [x] Role-deny checks pass (`viewer` mutating actions denied).
- [x] Cross-node consistency confirmed (same board/attachments on all 3 nodes).
- [x] Churn test passed (restart one node, then re-sync).

## 7) Observability
- [x] Logs are accessible and rotated. (logrotate installed + config rolled out on all nodes on 2026-02-15)
- [x] Metrics endpoint is reachable and scraped. (JSON metrics monitor timer rolled out on `srv1` on 2026-02-15)
- [x] Alerts configured for node down, sync failures, auth errors. (JSON monitor alerts to systemd journal + Prometheus rules present)

## 8) Release Artifacts
- [x] `CHANGELOG` updated.
- [x] Version tag prepared (`v0.x`).
- [x] Rollback plan documented (previous tag/commit + service restart steps).
- [x] Final smoke from `main` recorded in release notes.

---

## Go/No-Go Decision
- [x] GO
- [ ] NO-GO

Decision by: vova  
Timestamp (UTC): 2026-02-15  
Notes: Isolated contour with WG+VIP failover validated; MinIO x3, attachments, backup/restore, and operational monitoring baseline are active.
