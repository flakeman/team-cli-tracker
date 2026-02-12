# SDD-07 Plan

## Phase 1 - Rendering Contract
- Freeze canonical layout constants (column width, borders, header format).
- Document truncation and assignee fallback (`unassigned`) behavior.

## Phase 2 - Visual Hardening
- Refine padding/truncation for edge cases.
- Ensure row balancing when column lengths differ.
- Confirm stable output in live refresh mode.

## Phase 3 - Test Coverage
- Add table snapshot-style tests for representative states.
- Add edge-case tests for long unicode/multi-byte text.
- Add checks for header/count formatting.

## Phase 4 - Documentation
- Sync README board sample with canonical renderer.
- Link implementation status and evidence in tasks file.
