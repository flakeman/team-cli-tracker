# Operations: Backup/Restore And Token/Key Rotation

## 1) Data Backup

Create backup:
```bash
bash deploy/backup-data.sh /opt/team-cli-tracker/data /opt/team-cli-tracker/backup
```

Result:
- archive: `team-cli-data-<UTC>.tar.gz`
- checksum: `.sha256`

## 2) Data Restore (Drill)

Restore backup to target dir:
```bash
bash deploy/restore-data.sh /opt/team-cli-tracker/backup/team-cli-data-<UTC>.tar.gz /opt/team-cli-tracker/data-restore-drill
```

Post-restore integrity check:
```bash
go run ./cmd/node audit verify-integrity --data-dir /opt/team-cli-tracker/data-restore-drill
```

## 3) Token/Key Rotation Drill

Use temporary drill data dir:
```bash
TMP=$(mktemp -d)
go run ./cmd/node identity --node-id srv1 --data-dir "$TMP"
go run ./cmd/node issue create --node-id srv1 --data-dir "$TMP" --project-id OPS --issue-id OPS-ROTATE-1 --summary "rotation drill"
go run ./cmd/node storage enable-encryption --data-dir "$TMP"
go run ./cmd/node storage rotate-key --data-dir "$TMP" --max-age 720h
go run ./cmd/node storage recovery-drill --data-dir "$TMP"
```

Expected:
- `enable-encryption` => `ok`
- `rotate-key` => `ok`
- `recovery-drill` => `passed`

## 4) Log Rotation

Install template:
```bash
sudo cp deploy/logrotate-team-cli-tracker.conf /etc/logrotate.d/team-cli-tracker
sudo logrotate -d /etc/logrotate.d/team-cli-tracker
```

## 5) Monitoring/Alerts Baseline

Prometheus scrape template:
- `deploy/monitoring/prometheus-scrape.example.yml`

Alert rules:
- `deploy/monitoring/alerts-team-cli-tracker.rules.yml`

Prometheus integration:
1. include scrape config in main prometheus config
2. include alert rules in `rule_files`
3. reload prometheus and validate active targets/rules
