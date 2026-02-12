# SDD-01 Decentralized Kanban (P2P + Signed Events + CRDT/Raft)

## Context
Команде нужен канбан-трекер без обязательного VPS со статичным IP, с возможностью работать с разных компьютеров при динамических IP и защищенном обмене.

## Goal
Построить private decentralized tracker, где узлы участников синхронизируют задачи через P2P-сеть, а права/критичные переходы контролируются консенсусным контуром.

## Architecture
1. Transport: private overlay network (Tailscale/ZeroTier) + libp2p peer mesh.
2. Identity: node keypair + user keypair, подпись каждого события (Ed25519).
3. Data model: append-only event log per project.
4. Sync model: gossip + pull missing ranges by vector clock.
5. Conflict model: CRDT для не-критичных полей (comments, labels, checklist).
6. Authority model: Raft quorum for RBAC, workflow config, and protected transitions.
7. Storage: local embedded DB per node (SQLite/Bbolt) + snapshots.
8. Security: TLS in transport, signed payloads, replay protection by nonce/sequence.

## Why Not Bitcoin PoW
- PoW слишком дорогой по latency/cost.
- Не нужен permissionless trust model.
- Нужны приватность, RBAC и управляемая консистентность.

## Functional Requirements
1. Node join by invite token.
2. Project membership with roles (`admin`,`lead`,`dev`,`qa`,`viewer`).
3. Workflow transition validation (authoritative via Raft).
4. Kanban board view from local replicated state.
5. Offline writes queue with later sync.
6. Signed audit trail of all task mutations.
7. Per-project encryption keys rotation.

## Non-Functional Requirements
- p95 local read under 100ms.
- convergence after reconnect under 10s (small team baseline).
- no data loss on single node restart.
- tolerate one node failure in 3-node quorum.

## Deployment Modes
1. Home/LAN mode: all nodes in Tailscale mesh.
2. Hybrid mode: 1 lightweight public relay + home nodes.
3. VPS test mode: 3 Debian 13 nodes for quorum/chaos tests.

## MVP Scope (Phase 1)
- 3-node cluster bootstrap.
- Signed event append + replication.
- Basic issue create/transition/comment.
- CLI board view.
- Raft-guarded transition policy.

## Phase 2
- CRDT merge for concurrent edits.
- SLA reminders and subscriptions.
- Attachments sync via content-addressed blobs.

## Acceptance
- Node can rejoin after offline period and converge state.
- Unauthorized role cannot execute protected transition.
- Same project state visible from each node after sync.
- Audit chain verification passes on all nodes.
