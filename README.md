# team-cli-tracker

Decentralized CLI kanban tracker for teams working from different computers, including dynamic home IP scenarios.

## What This Project Is For
- Team task tracking from terminal (Linux/PowerShell/macOS).
- Private peer-to-peer collaboration without mandatory static public IP.
- Signed audit trail for task changes.
- Role-based workflow control for transitions (`admin`, `lead`, `dev`, `qa`, `viewer`).

## Core Architecture (Target)
- Private P2P overlay: Tailscale/ZeroTier + libp2p.
- Signed event log per project.
- CRDT merge for collaborative fields.
- Raft authority path for protected actions (RBAC/workflow policy).

## Repository Structure
- `cmd/node` - node process entrypoint.
- `internal/events` - signed event model.
- `internal/sync` - sync/vector clock primitives.
- `configs` - configuration examples.
- `docs` - SDD, roadmap, deployment runbooks.

## Current State
- MVP bootstrap skeleton is committed.
- SDD and roadmap are defined.
- Next milestone: first 3-node syncable cluster.

## Quick Start (Local)
```bash
go test ./...
go run ./cmd/node
```

## Docs
- Architecture spec: `docs/SDD-01-decentralized-kanban.md`
- Delivery roadmap: `docs/ROADMAP.md`
- 3-node deploy guide: `docs/DEPLOYMENT-DEBIAN13-3NODES.md`

## Near-Term Plan
1. Node identity and signed event append.
2. P2P peer discovery and state sync.
3. Basic issue commands: create, transition, comment.
4. CLI board rendering from replicated state.
5. Raft-protected transition validation.
