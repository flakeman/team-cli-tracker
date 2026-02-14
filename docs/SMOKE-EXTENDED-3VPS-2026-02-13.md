# Extended Smoke Report (`main`) - 2026-02-13 UTC

## Scope
Run extended operational validation on 3 VPS:
1. Batch workload of 36 issues (12 per node) with transitions/comments.
2. Command suite coverage for board/auth/team/trust/storage/audit/API.
3. Live operator observation path via `screen` on node `srv3-22223`.

## Environment
- Nodes:
  - `srv1.abuztech.ru (SSH port 22)` (`srv1-22221`)
  - `srv2.abuztech.ru (SSH port 22)` (`srv2-22222`)
  - `srv3.abuztech.ru (SSH port 22)` (`srv3-22223`)
- Binary: current `main` linux/amd64 build (`./node` in `~/team-cli-live`)

## Batch Workload
Per node (`srv1-22221`/`srv2-22222`/`srv3-22223`) executed:
- `12` issue creates
- `20` transitions
- `12` comments

Total across 3 nodes:
- `36` issues created
- `60` transitions
- `36` comments

Result: `PASS` (all create/transition/comment commands returned success).

## Board Snapshot Outcome
After batch on each node:
- Board renders correctly in table and counts-only modes.
- Column header semantics follow SDD-09:
  - `To Do [N]`
  - other columns `N/ALL`
- Totals lines present:
  - `Displayed issues`
  - `All issues`

## Command Suite Coverage (srv3-22223)
Validated successfully:
- `board`: `--once`, `--counts-only`, `--view mine`, `--interactive`
- `auth`: `issue`, `list`, `bind-role`, `list-bindings`, `revoke`
- `team`: `onboard`, `role-change`, `list`, `offboard`
- `trust`: `invite`, `list`, `revoke`
- `storage`: `key-policy-check`, `recovery-drill`, `verify-integrity`
- `audit`: export (`jsonl`/`csv`) and `verify-integrity`
- API checks:
  - `GET /healthz`
  - `GET /metrics`
  - `GET /security/audit`
  - `GET /security/audit/export`
  - `POST /issue/create`
  - `POST /issue/comment`
  - `GET /governance/list`

Result: `PASS` (no blocking errors in suite execution).

## Observation Mode
Interactive board process available on `srv3-22223` in screen session:
- `tct_board_int`
- attach with:
  - `screen -r tct_board_int`
