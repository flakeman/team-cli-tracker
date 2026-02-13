# SDD-08 Plan

## Phase 1 - Session Loop
- Add `--interactive` flag to `board`.
- Build redraw + input loop with graceful shutdown behavior.

## Phase 2 - Command Surface
- Implement command parser (`create/move/comment/view/counts/help/quit`).
- Reuse existing validators and event append paths.

## Phase 3 - UX Hardening
- Add status/error line and command hints.
- Ensure interactive mode coexists with `--refresh`, `--view`, `--assignee-id`.

## Phase 4 - Verification
- Add tests for parser and non-interactive backward compatibility.
- Add operator smoke script for interactive flow on VPS.
