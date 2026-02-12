# Smoke Checklist For `main` On 3 VPS

## Scope
Smoke validates current `main` baseline on 3-node deployment:
1. Auth issuer/token lifecycle.
2. Key policy + recovery drill.
3. Discovery/backoff/churn sanity.

## Prerequisites
1. Three reachable nodes with running `node serve`.
2. Admin token available for protected APIs.
3. `node` binary from current `main` available on each node.

## Inputs
- Node 1: `<host1>:<port1>`
- Node 2: `<host2>:<port2>`
- Node 3: `<host3>:<port3>`
- SSH user: `<user>`

## Steps

### A. Auth issuer/token lifecycle
1. Issue token:
   - `node auth issue --data-dir <data-dir> --user-id smoke-dev --role dev --ttl-sec 3600`
2. Use issued token against protected endpoint (expect authn success + authz deny for dev role):
   - `POST /raft/validate-transition` -> expect `403`.
3. Revoke token:
   - `node auth revoke --data-dir <data-dir> --token <issued-token>`
4. Retry protected endpoint with revoked token:
   - expect `401`.

### B. Key policy + recovery drill
1. Policy check:
   - `node storage key-policy-check --data-dir <data-dir> --max-age 720h`
2. Recovery drill:
   - `node storage recovery-drill --data-dir <data-dir>`
3. Verify audit evidence:
   - `GET /security/audit?limit=200`
   - assert event types:
     - `storage.key.policy_check`
     - `storage.key.recovery_drill`

### C. Discovery/backoff/churn sanity
1. Verify discovery metrics on each node:
   - `GET /metrics`
   - inspect:
     - `sync_discovered_peers`
     - `sync_peer_pulls`
     - `sync_peer_pull_success`
2. Simulate temporary peer outage (stop one node briefly), then rejoin.
3. After rejoin, assert:
   - `sync_peer_pull_success` resumes increasing.
   - no persistent divergence in board snapshots across nodes.

## Pass Criteria
1. Auth lifecycle checks produce expected codes (`403` then `401` after revoke).
2. Recovery drill passes and audit evidence exists.
3. Discovery/backoff metrics recover after node rejoin and board state converges.

## Evidence Capture
1. UTC timestamp of run.
2. Command outputs and HTTP status lines.
3. `/metrics` snapshots per node.
4. `/security/audit` extracts with relevant event types.
