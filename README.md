# team-cli-tracker

Decentralized CLI kanban tracker for teams.

Architecture direction:
- private P2P network (libp2p)
- signed event log
- CRDT for collaborative fields
- Raft authority for RBAC/workflow policy

## Next
- read docs/SDD-01-decentralized-kanban.md
- bootstrap first node binary in cmd/node
