# Contributing

## Quick Start
1. Fork and clone the repository.
2. Create a feature branch from `main`.
3. Run tests locally:
```bash
go test ./...
```
4. Open a Pull Request to `main`.

## Pull Request Rules
1. Keep PRs focused and small.
2. Update docs when behavior/contracts change.
3. Add or update tests for functional changes.
4. Fill `.github/pull_request_template.md`.

## Commit Style
- Use clear imperative messages, for example:
  - `feat: add master cluster audit export API`
  - `fix: enforce request id for master mutating endpoints`
  - `docs: align deployment and runtime contracts`

## Reporting Issues
- Use GitHub Issues and include:
  - expected behavior
  - actual behavior
  - reproduction steps
  - logs/commands (redact secrets)

## Security
- Do not open public issues for sensitive vulnerabilities.
- Use `SECURITY.md` reporting process.
