# Smoke Master API (3 VPS, 2026-02-15)

## Scope
- `master/health` on `srv1/srv2/srv3`
- `master/cluster/health` on `srv1/srv2/srv3`
- mutating `master/issues/create` with idempotency duplicate check
- mutating `master/team/onboard`

## Environment
- project: `OPS`
- nodes:
  - `srv1.example.internal:4101`
  - `srv2.example.internal:4101`
  - `srv3.example.internal:4101`
- auth tokens: `admin-token`, `lead-token`, `dev-token`

## Results
1. `GET /api/v1/master/health`:
- `srv1`: `200`, `status=ok`, `node_id=srv1`
- `srv2`: `200`, `status=ok`, `node_id=srv2`
- `srv3`: `200`, `status=ok`, `node_id=srv3`

2. `GET /api/v1/master/cluster/health`:
- `srv1`: `200`, `status=ok`, `peer_count=2`
- `srv2`: `200`, `status=ok`, `peer_count=2`
- `srv3`: `200`, `status=ok`, `peer_count=2`

3. `POST /api/v1/master/issues/create`:
- first request: `201 {"status":"created"}`
- second request with same `X-Request-Id`: `200 {"status":"duplicate", ...}`

4. `POST /api/v1/master/team/onboard`:
- `201 {"status":"onboarded", ...}`

## Acceptance
- Master bootstrap/cluster endpoints are reachable on all 3 nodes.
- Idempotency contract is active for mutating master endpoint.
- Team mutation via master alias succeeds.

