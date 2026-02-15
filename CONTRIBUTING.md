# Contributing

## Quick Start
1. Fork and clone the repository.
2. Create a feature branch from `main`.
3. Run tests locally:
```bash
go test ./...
```
4. Open a Pull Request to `main`.

## Development Setup
- Go toolchain version is defined in `go.mod`.
- Use repository defaults; avoid introducing environment-specific assumptions.
- Keep examples/docs anonymized (`example.internal`) for public artifacts.

## Pull Request Rules
1. Keep PRs focused and small.
2. Update docs when behavior/contracts change.
3. Add or update tests for functional changes.
4. Fill `.github/pull_request_template.md`.
5. Do not include secrets, private hostnames, or local credentials.

## Quality Gates
- Required before opening/merging PR:
```bash
go test ./...
```
- If you touched deployment/runtime docs, verify links and command examples.
- If you changed API behavior, update README and related SDD/docs.

## Commit Style
- Use clear imperative messages, for example:
  - `feat: add master cluster audit export API`
  - `fix: enforce request id for master mutating endpoints`
  - `docs: align deployment and runtime contracts`
- Prefer conventional prefixes: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`.

## Reporting Issues
- Use GitHub Issues and include:
  - expected behavior
  - actual behavior
  - reproduction steps
  - logs/commands (redact secrets)

## Scope For Contributions
Typical accepted changes:
- bug fixes and tests,
- docs/runbook accuracy improvements,
- API/CLI consistency fixes,
- operational hardening that preserves existing contracts.

Discuss larger architectural changes via an issue before implementation.

## Security
- Do not open public issues for sensitive vulnerabilities.
- Use `SECURITY.md` reporting process.
