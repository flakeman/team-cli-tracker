# Interactive Board Smoke Report (`main`) - 2026-02-13 UTC

## Scope
Validate interactive board operator flow (`node board --interactive`) on 3 VPS.

## Environment
- Nodes:
  - `srv1.abuztech.ru:22` (`srv1-22221`)
  - `srv2.abuztech.ru:22` (`srv2-22222`)
  - `srv3.abuztech.ru:22` (`srv3-22223`)
- OS: Ubuntu 24
- Binary: current `main` linux/amd64 build with interactive mode.

## Procedure (per node)
1. Start node service:
   - `node serve --project-id OPS --listen :4101 --data-dir <dir> --secure-mode-required=false`
2. Run scripted interactive session:
   - `help`
   - `create OPS-INT-<node> ...`
   - `move OPS-INT-<node> todo in_progress`
   - `comment OPS-INT-<node> ...`
   - `counts`
   - `view all`
   - `table`
   - `quit`
3. Capture board output and verify command status markers.

## Checks
All 3 nodes passed:
- `has_interactive=true`
- `has_help=true`
- `has_create_status=true`
- `has_move_status=true`
- `has_comment_status=true`
- `has_counts=true`
- `has_table=true`

## Result
`PASS` on `srv1-22221`, `srv2-22222`, `srv3-22223`. Interactive board accepts embedded commands and updates state/status without requiring a second terminal for command execution.
