# SDD-13: Isolated Network, Single Entrypoint HA, And S3 Storage Baseline

## Context
Project moved from ad-hoc VPS test operations to a reproducible production-style isolated contour:
- private node-to-node connectivity,
- one stable client entrypoint,
- resilient failover,
- durable attachment storage.

Existing deployment docs and scripts cover parts of this, but the scope needs formal SDD acceptance contract.

## Goal
Define and lock a production baseline for isolated deployments with:
1. WireGuard mesh between nodes (`10.20.0.0/24`).
2. Gateway on every node.
3. Single client entrypoint `board.internal` with HA failover.
4. S3-compatible attachment backend (distributed MinIO x3) as default.
5. `local` attachment backend preserved as dev fallback.

## In Scope
- Deployment contracts (`deploy/*.env.example`, install scripts).
- Runtime contracts (`systemd` services for tracker, gateway, wireguard, minio, keepalived).
- HA entrypoint options:
  - internal L4/L7 LB,
  - VRRP/keepalived VIP,
  - DNS multi-A + health-check.
- Node lifecycle baseline for scaling (`10.20.0.N` addressing model).
- Release acceptance criteria and smoke evidence requirements.

## Out of Scope
- UI/frontend redesign.
- SaaS tunnel dependencies for isolated mode.
- Full auto-orchestration platform (K8s/nomad).

## Architecture Contract
### Networking
- Node overlay transport: WireGuard (`wg0`).
- Node VPN IP examples:
  - `srv1=10.20.0.1`
  - `srv2=10.20.0.2`
  - `srv3=10.20.0.3`
- Next nodes continue sequence (`10.20.0.4+`).

### Entrypoint
- Client target hostname: `board.internal`.
- Gateway runs on all nodes.
- Entrypoint failover is mandatory via one of:
  - LB,
  - VRRP VIP (recommended default),
  - DNS health-aware multi-A.

### Storage
- Baseline attachment backend in prod: `s3`.
- Baseline S3 implementation: distributed MinIO x3.
- Stable storage endpoint: `s3.internal`.
- Bucket baseline: `team-cli-attachments`.
- Fallback for dev: `attachment-backend=local`.

## Security/Operations Contract
- `secure-mode-required=true` in production.
- Node ports not exposed publicly in isolated mode.
- Single entrypoint only.
- SSH key-based access preferred; password access restricted.
- Health checks required for:
  - tracker,
  - gateway,
  - MinIO,
  - VRRP state.

## Acceptance Criteria
1. WireGuard active and peer-reachable on all nodes.
2. Gateway active on all nodes.
3. `board.internal` remains reachable when one gateway node is down.
4. Tracker remains operational under single node failure (quorum behavior validated).
5. Attachment flow in S3 mode passes:
   - initiate/upload/complete/verify.
6. Attachment visibility/integrity consistent from all nodes.
7. Churn test:
   - restart one tracker node and one gateway node,
   - confirm resync and entrypoint continuity.
8. Release checklist updated and marked GO/NO-GO accordingly.

## Deliverables
- SDD-13 plan/tasks.
- Deploy scripts and env templates.
- Isolated deployment runbook.
- Smoke evidence for network, failover, attachments, churn.
