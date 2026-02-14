# Smoke Report: Attachments + Webhooks (3 VPS, main)

- Date: 2026-02-13
- Scope:
  - attachment file flow (`initiate -> upload -> complete -> open`)
  - attachment integrity checks (`verify`, `verify-all`)
  - webhook management and delivery (`create/list/test/revoke`)
  - API namespace parity (`/api/v1/*`)

## Preconditions
- 3-node cluster topology is active:
  - `srv1.abuztech.ru:22` (`srv1-22221`)
  - `srv2.abuztech.ru:22` (`srv2-22222`)
  - `srv3.abuztech.ru:22` (`srv3-22223`)
  for project `OPS`.
- Secure auth token used for protected API routes.
- Attachment backend tested in both modes:
  - `local` (default),
  - `s3` (MinIO/S3 via `--attachment-backend s3`).

## Executed Checks
1. `POST /api/v1/issue/attachment/initiate` returns `attachment_id` and upload URL.
2. Upload by returned URL completes.
3. `POST /api/v1/issue/attachment/complete`:
   - accepts valid checksum,
   - rejects checksum mismatch.
4. `POST /api/v1/issue/attachment/open` returns download URL and checksum.
5. `POST /api/v1/issue/attachment/verify` returns deterministic match result.
6. `GET /api/v1/issue/attachment/verify-all` returns project-level summary.
7. Webhook lifecycle:
   - create/list/test/revoke endpoints are functional,
   - attachment add/remove emits webhook event payload.
8. Periodic integrity scheduler:
   - leader-policy scheduling active,
   - metrics expose `attachment_verify_*` fields.

## Result
- Status: PASS
- No blocking issues detected in attachment API/webhook baseline.

## Evidence
- API examples and operator commands are documented in `README.md`.
- Feature implementation and tests are on `main`:
  - `f909b43`, `aad6c09`, `84d6384`, `1365c4a`, and subsequent webhook commit.
