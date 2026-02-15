# Project Checkpoint (2026-02-15)

## Git State
- Branch: `main`
- HEAD: `081621b`
- Worktree: clean
- Tags: `v0.1.0`, `v0.8.0`, `v0.8.1`, `stable`, `latest`

## Implemented (high-level)
1. Master API control-plane baseline and parity aliases.
2. Master mutating idempotency (`X-Request-Id` required).
3. Cluster audit aggregation:
   - CLI: `node audit cluster-export`
   - API: `GET /api/v1/master/audit/cluster-export`
4. Production runtime hardening on 3 nodes (`systemd`, secure mode, TLS/mTLS wiring in live environment).
5. S3/MinIO attachment flow (`initiate -> upload -> complete -> verify`) with checksum model.
6. Documentation cleanup and anonymization (`*.example.internal`).
7. GitHub baseline files added:
   - `LICENSE`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`
   - issue templates
   - CI workflow: `.github/workflows/go-ci.yml`

## Current Documentation Shape
- Main entrypoint: `README.md`
- Documentation index by tags: `docs/INDEX.md`
- Key operations:
  - `docs/DEPLOYMENT-DEBIAN13-3NODES.md`
  - `docs/DEPLOYMENT-ISOLATED-WG-HA.md`
  - `docs/RELEASE-CHECKLIST.md`
  - `docs/ROLLBACK-RUNBOOK.md`
  - `docs/SECURITY-RUNBOOK.md`

## Runtime Model (current)
- Tasks: derived from append-only event log (`data-dir`), not JSON task files.
- Attachments:
  - binary content in S3/MinIO
  - metadata + checksums in event log
- Audit:
  - local per node (`security_audit.jsonl`)
  - merged cluster view via `master/audit/cluster-export`

## Known Non-Critical Follow-ups
1. Optional CI job for doc-link validation.
2. Optional CI profile for multi-node e2e smoke (3 nodes + S3 + cluster audit export).
3. Optional further README reduction (if needed for public landing simplicity).

## Last Verified Locally
- `go test ./...` passes.

