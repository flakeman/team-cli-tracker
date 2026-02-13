# SDD-09 Board Counting Semantics And Interactive Input Cadence

## Context
After `SDD-08` interactive mode rollout, two operator UX gaps were identified:
1. Column counts were interpreted ambiguously.
2. Periodic redraw could interfere with command typing in interactive sessions.

## Goal
Standardize board counting semantics and input cadence so operators can reliably read board metrics and type commands without redraw disruption.

## Scope
1. Board column header counting contract.
2. Global totals and scope visibility (`all` vs `mine`).
3. Interactive refresh control behavior (`interactive-refresh`, `pause/resume`).
4. Documentation and regression coverage for these rules.

## Out Of Scope
- Workflow policy changes.
- New status columns.
- Multi-project aggregation.

## Functional Requirements
1. Header counting format:
   - `To Do [N]` (absolute count only).
   - `In Progress [N/ALL]`, `Code Review [N/ALL]`, `Testing [N/ALL]`, `Done [N/ALL]`.
2. `ALL` is the total number of issues on the board scope baseline (full board total).
3. Board top section must include:
   - `Scope: all|mine(<assignee>)`
   - `Displayed issues: ...`
   - `All issues: ...`
4. Interactive mode must support redraw/input decoupling:
   - `--interactive-refresh 0s` disables periodic redraw.
   - `pause`/`resume` commands toggle periodic redraw in-session.
5. Existing non-interactive board usage must remain backward compatible.

## Non-Functional Requirements
- Counting output must be deterministic for identical state.
- Interactive mode must remain responsive while typing commands.
- No terminal corruption on repeated mode switching (`counts`/`table`, `pause`/`resume`).

## Acceptance
1. Column headers match agreed format (`To Do [N]`, others `N/ALL`).
2. Users can clearly distinguish displayed vs total issue counts.
3. Interactive sessions are usable with refresh disabled or paused.
4. Tests and docs reflect final counting and cadence contract.
