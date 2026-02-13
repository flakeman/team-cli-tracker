# Engineering Process

## Debt Review Cadence
- Quarterly: review `docs/TECH-DEBT.md` statuses and priorities.
- On every architecture/security/state PR: update debt register delta.

## Required PR Checks (architecture-affecting)
1. Update `docs/TECH-DEBT.md` if behavior/risk changed.
2. Link evidence for any item moved to `Closed`:
   - tests,
   - smoke report,
   - API/CLI docs update.
3. Update roadmap/SDD references when milestone scope shifts.

## Debt Closure Rule
- A debt item can be closed only when all exit criteria in `docs/TECH-DEBT.md` are demonstrably met.

## Recordkeeping
- Keep release checklist in sync with debt reality:
  - `docs/RELEASE-CHECKLIST.md`
- Keep README command examples aligned with implemented behavior.
