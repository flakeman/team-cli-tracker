# Board Live Smoke Report (`main`) - 2026-02-12 UTC

## Scope
Validate active board updates in live mode (`node board --refresh 1s`) on 3 VPS.

## Environment
- Nodes:
  - `srv1.abuztech.ru (SSH port 22)` (`srv1-22221`)
  - `srv2.abuztech.ru (SSH port 22)` (`srv2-22222`)
  - `srv3.abuztech.ru (SSH port 22)` (`srv3-22223`)
- OS: Ubuntu 24
- Build: `main` commit `8fd7613` (`linux/amd64`)

## Procedure (per node)
1. Start local node:
   - `node serve --project-id OPS --listen :5210 --data-dir <dir> --secure-mode-required=false`
2. Start board live capture:
   - `timeout 8s node board --project-id OPS --data-dir <dir> --refresh 1s`
3. While board is running:
   - create issue `OPS-LIVE-1`
   - transition `todo -> in_progress`
4. Verify captured output file contains both initial and updated states.

## Evidence
All 3 nodes produced identical checks:
- `project=true`
- `rev0=true`
- `rev1=true`
- `todo1=true`
- `inprog1=true`
- `opslive=true`
- output length: `7520` bytes per node

Observed canonical table lines:
- `Project: OPS  Revision: 0`
- `Assignee WIP limit: 3`
- `To Do [0] ... In Progress [0] ...`
- after mutation:
  - `Project: OPS  Revision: 1`
  - `To Do [1]`
  - `In Progress [1]`
  - card containing `OPS-LIVE-1`

## Result
`PASS` on all 3 VPS. Live board reflects added/moved task state during active refresh without manual re-run.
