# Rollback Runbook

## Scope
Rollback tracker runtime and deployment scripts to previous known-good commit/tag.

## Preconditions
- SSH access to all nodes.
- Systemd-managed runtime (`team-cli-tracker.service` or `team-cli-rerun.service`).
- Target rollback ref known (`<tag>` or `<commit_sha>`).

## 1) Identify target ref
```bash
cd /opt/team-cli-tracker
git fetch origin --tags
git log --oneline -n 20
```

## 2) Rollback one node (canary)
```bash
cd /opt/team-cli-tracker
git checkout <target_ref>
sudo systemctl restart team-cli-tracker || sudo systemctl restart team-cli-rerun
systemctl is-active team-cli-tracker || systemctl is-active team-cli-rerun
curl -sS http://127.0.0.1:4101/healthz
```

## 3) Validate canary
- Health is `ok`.
- Board/API basic operations work.
- No critical errors in service log:
```bash
sudo journalctl -u team-cli-tracker -n 100 --no-pager || sudo journalctl -u team-cli-rerun -n 100 --no-pager
```

## 4) Rollback remaining nodes
Repeat step 2 for each remaining node.

## 5) Post-rollback checks
1. Cross-node consistency:
   - same board revision trend,
   - key issues visible on all nodes.
2. Attachment verify smoke:
```bash
curl -s "http://127.0.0.1:4101/api/v1/issue/attachment/verify-all?project_id=OPS" -H "Authorization: Bearer admin-token"
```
3. Audit export smoke:
```bash
go run ./cmd/node audit export --all --format jsonl > /tmp/audit-post-rollback.jsonl
```

## 6) Roll-forward after fix
```bash
cd /opt/team-cli-tracker
git fetch origin
git checkout main
git pull --ff-only origin main
sudo systemctl restart team-cli-tracker || sudo systemctl restart team-cli-rerun
```
