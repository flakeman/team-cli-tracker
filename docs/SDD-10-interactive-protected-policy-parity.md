# SDD-10 Interactive Command Parity With Protected Policy Flow

## Context
Interactive board commands are useful for operators, but currently they can bypass the same protected policy path used in secured API workflows.

## Goal
Bring interactive command execution closer to production-protected behavior, starting with transition validation parity.

## Scope
1. Policy-aware interactive transitions (`move`) via protected validation path.
2. Explicit mode contract for interactive commands (local append vs protected validation).
3. Regression tests for policy-accept/policy-deny in interactive path.

## Out Of Scope
- Full remote CRUD migration for all interactive commands in this iteration.
- External auth provider changes.

## Functional Requirements
1. Interactive `move` must support policy validation endpoint before local append.
2. Policy URL must be configurable (`--interactive-policy-url` / env fallback).
3. On policy deny, command returns clear error and does not append transition event.
4. Help/docs must explicitly describe protected mode behavior.

## Non-Functional Requirements
- No regression for local-only development mode.
- Clear operator feedback for allow/deny cases.

## Acceptance
1. `move` in interactive mode follows same validation semantics as non-interactive protected transition path.
2. Tests cover allow and deny behavior.
3. TD-009 current state is updated with evidence.
