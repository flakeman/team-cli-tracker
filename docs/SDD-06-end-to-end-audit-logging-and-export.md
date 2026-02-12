# SDD-06 End-To-End Audit Logging And Export

## Context
Current audit coverage is event-centric and partially command-scoped. Operations needs a single, end-to-end action log across CLI/API flows, with reliable export for compliance and incident response.

## Goal
Introduce unified end-to-end logging for all user and system actions, and provide export modes:
1. Full export (all records).
2. Period export (time range).
3. User-filtered export (one or many users).

## Scope
1. Unified audit schema for commands, API mutations, and internal critical system actions.
2. Durable append-only storage for audit records with hash-chain continuity.
3. CLI/API export endpoints for full/time-range/user filters.
4. Operational safeguards: pagination, integrity verification, and redaction policy.

## Out Of Scope
- External SIEM integration connectors in this iteration.
- Real-time streaming transport (webhook/Kafka).

## Functional Requirements
1. Every state-changing action must generate an audit record with:
   - `event_id`, `event_time_utc`, `actor_user_id`, `actor_role`, `source` (cli/api/system),
   - `action`, `resource_type`, `resource_id`, `project_id`,
   - `result` (success/deny/error), `reason`,
   - `request_id`/correlation id,
   - `prev_hash`, `hash`.
2. Read-only sensitive actions (auth/token/trust/security) must also be logged.
3. Export interfaces:
   - `node audit export --all`
   - `node audit export --from <RFC3339> --to <RFC3339>`
   - `node audit export --user <id[,id2,...]>`
4. Exports must support `jsonl` and `csv` formats.
5. Export operations must be auth-protected and produce their own audit record.
6. Query filters can be combined:
   - period + user,
   - period-only,
   - user-only,
   - all.
7. Large exports must use stable ordering (`event_time_utc`, `event_id`) and pagination cursor.

## Non-Functional Requirements
- Audit write path overhead: p95 < 20ms per action in 3-node baseline.
- Export of 100k records should complete without process OOM.
- Hash-chain verification command must detect deletion/reordering/tampering.

## Data And Security Requirements
1. Timestamps stored in UTC only.
2. Sensitive fields (tokens, secrets, private keys) must be redacted at write-time.
3. Retention policy configurable (`AUDIT_RETENTION_DAYS`), default `365`.
4. Optional signed export manifest with checksum for offline verification.

## Acceptance
1. Any create/transition/comment/team/trust/governance action appears in audit with correlation id.
2. `--all`, `--from/--to`, and `--user` exports return correct deterministic subsets.
3. Combined filters (period + users) work and are covered by tests.
4. Redaction rules prevent leakage of secrets in stored logs and exported files.
5. Integrity check fails on modified/tampered audit dataset and passes on original.
