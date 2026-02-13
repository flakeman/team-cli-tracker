# SDD-08 Board Interactive Mode And Operator Flow

## Context
Board rendering and live updates are already stable (`SDD-07`). Operators now need a single-session workflow: watch board and execute task actions without switching terminal windows.

## Goal
Deliver interactive board mode with embedded command input, while preserving existing non-interactive flows (`--once`, `--refresh`, `--view`, `--counts-only`).

## Scope
1. Interactive board session mode for plain format.
2. Embedded command parser for core task operations.
3. Status/help/error panel in interactive loop.
4. Compatibility with existing view modes (`all`/`mine`) and counts summary.

## Out Of Scope
- Full-screen TUI framework migration.
- Mouse support and advanced keybindings.
- Multi-project dashboard in one screen.

## Functional Requirements
1. New flag:
   - `node board --interactive`
2. Interactive mode supports commands:
   - `create <ISSUE_ID> <summary...>`
   - `move <ISSUE_ID> <from> <to>`
   - `comment <ISSUE_ID> <text...>`
   - `view all|mine [assignee]`
   - `counts`
   - `help`
   - `quit`
3. Interactive mode keeps periodic board refresh active while accepting commands.
4. Command execution result is shown in status line and logged in audit.
5. Invalid commands must not crash session; show parse/validation error and continue.

## Non-Functional Requirements
- Input-to-feedback latency p95 < 300ms for local actions.
- No terminal corruption after repeated redraw + command input.
- Graceful exit on `quit`, `Ctrl+C`, `SIGTERM`.

## Acceptance
1. Operator can create and move task from the same board session.
2. Mode switching (`view all/mine`) updates table without restart.
3. Counts summary is accessible in-session via `counts`.
4. Existing non-interactive commands behave unchanged.
