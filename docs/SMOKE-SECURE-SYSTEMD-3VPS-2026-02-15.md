# Smoke: Secure systemd runtime (3 VPS)

Date: 2026-02-15  
Branch: `main`  
Operator: `vova`

## Scope
- `team-cli-rerun.service` runs under `systemd` on `srv1/srv2/srv3`.
- `secure-mode-required=true` with TLS/mTLS flags in service command.
- Local node health and master control-plane health.

## Result
- `srv1`: `active`, `https://127.0.0.1:4101/healthz` => `{"status":"ok"}`
- `srv2`: `active`, `https://127.0.0.1:4101/healthz` => `{"status":"ok"}`
- `srv3`: `active`, `https://127.0.0.1:4101/healthz` => `{"status":"ok"}`
- `/api/v1/master/health` => `status=ok` on all 3 nodes.
- `/api/v1/master/cluster/health` => `status=ok`, `peer_count=2` on all 3 nodes.

## Notes
- Root cause of previous instability: `systemd` unit used insecure/missing TLS inputs for `secure-mode-required=true`.
- Runtime now uses explicit TLS files per node and CA trust for outgoing peer requests.
