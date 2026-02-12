# SDD-02 Plan

## Phase A - Identity And Signed Log
1. Node key management and stable `node_id`.
2. Signed event envelope and verifier.
3. Append-only store with sequence/replay checks.

## Phase B - P2P Sync
1. Peer discovery bootstrap.
2. Vector-clock based gap detection.
3. Missing-range fetch and replay apply.

## Phase C - Task Commands And Board
1. CLI commands: `issue create`, `issue transition`, `issue comment`.
2. Local projection model for issues/comments/status.
3. CLI board renderer from replicated projection.

## Phase D - Protected Transitions
1. Raft-backed workflow policy validator.
2. Transition authorization path and quorum errors.
3. Admin-preferred leader mode with automatic failover.

## Phase E - Hardening
1. Integration tests (multi-node converge + policy checks).
2. Observability logs/metrics for sync and consensus path.
3. Runbook updates for troubleshooting.

