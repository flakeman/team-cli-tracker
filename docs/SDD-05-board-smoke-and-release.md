# SDD-05 Board UX, 3-VPS Smoke, And Release Baseline

## Context
Core technical debt is closed in `docs/TECH-DEBT.md`. The next milestone is production-facing quality:
1. Make `board` output match the intended tabular UX from `README.md`.
2. Validate current `main` on real 3-VPS environment with security and resilience smoke checks.
3. Formalize release hygiene (changelog and version tags).

## Goal
Move from "technically complete core" to "operator-ready baseline" by aligning CLI UX, validating real deployment behavior, and producing release artifacts.

## Scope
1. Board rendering overhaul in CLI plain output.
2. End-to-end smoke runbook and execution checklist for 3 VPS.
3. Release process artifacts (`CHANGELOG`, version tag policy, release checklist).

## Out Of Scope
- New UI frontend/web dashboard.
- Major protocol redesign.
- Multi-environment CI/CD orchestration.

## Functional Requirements
1. `node board --format plain` prints fixed-width multi-column table aligned with workflow columns.
2. Board output includes:
   - Project/revision header.
   - WIP limits and per-column counts.
   - Stable, readable row wrapping/truncation policy.
3. 3-VPS smoke checklist validates:
   - Auth issuer lifecycle (`issue/revoke/list/bind-role`).
   - Key policy and recovery drill.
   - Discovery/backoff/churn behavior.
4. Release artifacts include:
   - Human-readable changelog entries grouped by area.
   - `v0.x` semantic tag and release notes template.

## Non-Functional Requirements
- Board rendering remains deterministic across repeated calls on same state.
- Smoke checklist execution should complete in <= 30 minutes on a healthy 3-node cluster.
- Release checklist must be reproducible by another operator without hidden steps.

## Acceptance
1. `board` plain output visually matches SDD sample style (column table, counts, limits).
2. 3-VPS smoke run passes on `main` with recorded evidence.
3. Release checklist, changelog, and `v0.x` tag are prepared and documented.
