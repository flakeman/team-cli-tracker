# SDD-13 Plan

## Workstreams
1. WireGuard Baseline
- Finalize env contract (`wireguard.env`).
- Validate install script and health verification commands.
- Standardize node IP allocation model (`10.20.0.N`).

2. Multi-Gateway + Single Entrypoint
- Ensure gateway config supports node-local binding.
- Provide and document 3 HA entrypoint modes.
- Set VRRP/keepalived as recommended default for isolated contour.

3. S3/MinIO Baseline
- Add MinIO deployment scripts and systemd service.
- Add bootstrap and health-check scripts.
- Wire tracker runtime to S3 env settings.
- Preserve local backend fallback.

4. Runtime Wiring
- Ensure `install-systemd-service.sh` consumes cluster env contract.
- Validate `secure-mode-required=true` default.
- Validate peer URLs and runtime flags serialization.

5. Documentation + Release Gates
- Update isolated runbook with WG + gateway + MinIO + failover flow.
- Extend release checklist with failover and S3 health gates.
- Add changelog entry for baseline.

## Validation Plan
1. Static:
- `bash -n` for new/changed deploy scripts.
- Docs reference consistency checks.

2. Integration:
- Bring up services on 3 nodes:
  - wg, tracker, caddy, minio, keepalived.
- Verify:
  - `board.internal` availability,
  - failover continuity,
  - S3 attachment lifecycle.

3. Churn:
- restart tracker on one node,
- restart gateway on one node,
- assert continuity and consistency.

## Exit Criteria
- All SDD-13 acceptance items pass with dated smoke evidence.
- Checklist items for entrypoint failover and S3 storage marked complete.
