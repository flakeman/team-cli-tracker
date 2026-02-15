# Resume Notes

## Where to Continue
1. Open checkpoint: `docs/PROJECT-CHECKPOINT-2026-02-15.md`
2. Open docs index: `docs/INDEX.md`
3. Open main readme: `README.md`

## Quick Validation Commands
```bash
go test ./...
```

## Quick Runtime Commands (local)
```bash
# run node
go run ./cmd/node serve --project-id OPS --listen :4101 --data-dir ./data --secure-mode-required=false

# board
go run ./cmd/node board --project-id OPS

# audit cluster export example
go run ./cmd/node audit cluster-export --data-dir ./data --self-node-id srv1 --peers https://srv2.example.internal:4101,https://srv3.example.internal:4101 --auth-token admin-token --all --format csv --insecure-tls
```

## Context Markers
- Master cluster audit API exists: `/api/v1/master/audit/cluster-export`
- Master mutating endpoints require `X-Request-Id`
- Attachment binaries are S3/MinIO-backed

