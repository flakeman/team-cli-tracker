# Release Checklist (Go/No-Go)

Date: 2026-02-15  
Release: pre-v0.x  
Owner: vova

## 1) Network
- [x] Inter-node `4101/tcp` is reachable both directions: `srv1 <-> srv2 <-> srv3`.
- [ ] Public/consumer entrypoint is defined (`board DNS` or local-only mode). (pending decision)
- [x] Single entrypoint failover is configured (LB or VRRP VIP or DNS health-check).
- [~] SSH keys are configured; password SSH is disabled or restricted. (keys configured between nodes, password access still enabled)

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
- [ ] TLS/mTLS decision is documented and applied (or private-trust boundary is approved).

## 4) Data and Recovery
- [x] Backup policy for `data-dir` exists (schedule + retention). (documented via backup/restore runbook + scripts)
- [x] Restore drill was executed successfully. (srv1 drill with archive restore and board projection check on 2026-02-15)
- [x] Audit export works (`all`, `period`, `users`).
- [x] Audit integrity verification passes.

## 5) Attachments
- [x] Attachment backend is selected (`local` or `s3`) and documented. (prod baseline: `s3`, dev fallback: `local`)
- [x] Distributed MinIO (`x3`) is deployed and healthy.
- [ ] `s3.internal` endpoint is stable via selected HA profile.
- [x] `initiate -> upload -> complete -> verify` flow passes.
- [x] `verify-all` passes for current project.
- [ ] Storage capacity/cleanup policy exists.

## 6) Functional E2E
- [x] 30-40 issues scenario executed. (40 issues bulk run completed on 2026-02-15)
- [~] All core commands tested: create, move, comment, archive/unarchive, board modes. (archive/unarchive + full board mode matrix still pending)
- [x] Role-deny checks pass (`viewer` mutating actions denied).
- [x] Cross-node consistency confirmed (same board/attachments on all 3 nodes).
- [x] Churn test passed (restart one node, then re-sync).

## 7) Observability
- [~] Logs are accessible and rotated. (logrotate template added; rollout on all nodes pending)
- [~] Metrics endpoint is reachable and scraped. (Prometheus scrape template added; live scraper rollout pending)
- [~] Alerts configured for node down, sync failures, auth errors. (Prometheus alert rules added; Alertmanager wiring pending)

## 8) Release Artifacts
- [x] `CHANGELOG` updated.
- [ ] Version tag prepared (`v0.x`).
- [x] Rollback plan documented (previous tag/commit + service restart steps).
- [x] Final smoke from `main` recorded in release notes.

---

## Go/No-Go Decision
- [ ] GO
- [x] NO-GO

Decision by: vova (pending final sign-off)  
Timestamp (UTC): 2026-02-15  
Notes: Systemd + secure mode + 40-issue/churn/users/files/sync validated; release gate still blocked by unresolved security/ops/release artifact items.
