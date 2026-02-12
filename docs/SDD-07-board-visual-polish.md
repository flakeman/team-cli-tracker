# SDD-07 Board Visual Polish And Readability Contract

## Context
Current board output is functional and live-updating, but visual consistency still varies by terminal width, long summaries, and dense task sets. The team needs a strict rendering contract so board view always matches the target style shown in README.

## Goal
Stabilize board plain rendering to a predictable, readable, production-grade terminal table:
1. Exactly five workflow columns with fixed geometry.
2. Clean headers and counts in each redraw.
3. Consistent truncation/alignment rules for task cards.

## Scope
1. Finalize table geometry and spacing for:
   - project header line,
   - WIP line,
   - border/header/body rows.
2. Define deterministic overflow behavior for:
   - long issue ids,
   - long summaries,
   - long assignee ids.
3. Add snapshot-like tests for visual regressions.
4. Document style contract for RU/EN examples in README.

## Out Of Scope
- Rich TUI framework migration.
- ANSI coloring/themes.
- Interactive keyboard navigation.

## Functional Requirements
1. Board plain mode must render this canonical structure on every refresh:
   - `Project: <id>  Revision: <n>`
   - `Assignee WIP limit: <n>`
   - top border, column header row, separator, data rows, bottom border.
2. Columns must always be ordered:
   - `To Do`, `In Progress`, `Code Review`, `Testing`, `Done`.
3. Header cells must include count and WIP cap (or `inf`) in format:
   - `<Column> [<count>/<cap>]`
4. Card line format must be:
   - `<ISSUE-ID> <summary> @<assignee-or-unassigned>`
5. Truncation policy:
   - hard max width per cell,
   - ellipsis when overflow,
   - no broken table borders.
6. Empty cells in sparse columns must preserve vertical alignment.

## Non-Functional Requirements
- Rendering must be deterministic for identical event state.
- Live refresh must not introduce flicker artifacts beyond full-screen redraw.
- No panic/format corruption on non-ASCII summaries.

## Acceptance
1. Output visually matches canonical README example style.
2. Regression tests cover:
   - empty board,
   - long summary/assignee,
   - mixed row heights across columns.
3. Live mode keeps alignment stable across repeated updates.
