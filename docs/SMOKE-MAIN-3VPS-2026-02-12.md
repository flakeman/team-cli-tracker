# Smoke Execution Report (`main`) - 2026-02-12 UTC

## Environment
- Hosts / nodes:
  - node-1: `srv1.example.internal (SSH port 22)` (`srv1-22221`)
  - node-2: `srv2.example.internal (SSH port 22)` (`srv2-22222`)
  - node-3: `srv3.example.internal (SSH port 22)` (`srv3-22223`)
- OS: Ubuntu 24
- Binary: linux amd64 build from current `main`

## Scope
1. Auth issuer and token lifecycle.
2. Key policy check and recovery drill.
3. Discovery/backoff/churn sanity.

## Results

### 1. Auth issuer/token lifecycle
- `auth issue` produced token length `48`.
- Protected endpoint check with issued `dev` token: `403` (authn success, authz deny).
- `auth revoke` returned `200`.
- Reuse of revoked token returned `401`.
- Result: `PASS` on all 3 nodes.

### 2. Key policy and recovery drill
- `storage key-policy-check` returned: `ok: encryption is not enabled`.
- `storage recovery-drill` returned: `ok: recovery drill passed ... events_checked=0`.
- Audit check:
  - `storage.key.recovery_drill`: present.
  - `storage.key.policy_check`: missing when encryption is disabled (current behavior).
- Result: `PASS WITH NOTE` (policy-check event emission depends on encryption state).

### 3. Discovery/backoff/churn sanity
- Before temporary outage:
  - `sync_peer_pull_success=3`
  - `sync_peer_skipped_backoff=0`
- During outage:
  - `sync_peer_pull_success=3`
  - `sync_peer_skipped_backoff=1`
- After rejoin:
  - `sync_peer_pull_success=6`
  - `sync_peer_skipped_backoff=2`
  - `sync_discovered_peers=0` in this topology.
- Result: `PASS` (backoff and pull recovery observed).

## Conclusion
- Smoke passed for baseline auth lifecycle and peer recovery behavior.
- Follow-up recommended: emit `storage.key.policy_check` audit event even when encryption is disabled, for stricter evidence parity.

