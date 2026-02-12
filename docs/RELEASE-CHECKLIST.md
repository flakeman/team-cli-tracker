# Release Checklist (`v0.x`)

## Versioning Policy
1. Use semantic tags: `v0.x.y`.
2. `x` increments for feature milestones, `y` for patch-only releases.
3. Tag must point to a tested commit on `main`.

## Pre-Tag Checks
1. `go test ./...` passes.
2. 3-VPS smoke (`docs/SMOKE-MAIN-3VPS.md`) passes with recorded evidence.
3. `docs/TECH-DEBT.md` status reflects current reality.
4. `CHANGELOG.md` updated for target version.
5. Security-sensitive env vars/commands reviewed in docs.

## Tagging
1. Create annotated tag:
   - `git tag -a v0.x.y -m "v0.x.y"`
2. Push tag:
   - `git push origin v0.x.y`

## Release Notes
1. Copy template from `docs/RELEASE-NOTES-TEMPLATE.md`.
2. Fill:
   - Summary
   - Highlights
   - Breaking/behavioral changes
   - Verification notes

## Post-Release
1. Verify tag visibility on GitHub.
2. Pin release note link in team channel/runbook.
3. Open follow-up tasks for known non-blocking issues.
