# SDD-06 Plan

## Phase 1 - Unified Audit Contract
- Align action taxonomy (`action`, `resource`, `result`, `actor`).
- Normalize timestamps to RFC3339 UTC.
- Define required redaction rules for secrets and tokens.

## Phase 2 - Export Interfaces
- Add CLI export command for:
  - full export,
  - period export,
  - user-filtered export.
- Support `jsonl` and `csv` output formats.
- Add combined filtering (period + users).

## Phase 3 - Coverage Expansion
- Ensure all critical mutation paths emit audit records.
- Add export audit record (`audit.export`) for traceability.
- Validate deterministic ordering and stable pagination basis.

## Phase 4 - Verification
- Add unit tests for filter correctness and edge cases.
- Add smoke checks for export commands on multi-node environment.
- Finalize operational docs for retention and integrity verification.
