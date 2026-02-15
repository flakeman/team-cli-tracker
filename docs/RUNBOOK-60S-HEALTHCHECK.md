# 60-Second Healthcheck Runbook

Date: 2026-02-15  
Target: 3-node isolated contour (`srv1/srv2/srv3`) with WG + VIP + MinIO

## 1) App health (all nodes)
Run from any node with SSH access:

```bash
ssh vova@srv1.example.internal 'curl -fsS http://127.0.0.1:4101/healthz'
ssh vova@srv2.example.internal 'curl -fsS http://127.0.0.1:4101/healthz'
ssh vova@srv3.example.internal 'curl -fsS http://127.0.0.1:4101/healthz'
```

Expected: each returns `{"status":"ok" ...}`.

## 2) VIP entrypoint

```bash
curl -fsS http://10.20.0.10:8080/api/v1/healthz
```

Expected: `{"status":"ok" ...}`.

## 3) VIP owner check

```bash
ssh vova@srv1.example.internal "sudo ip a show dev wg0 | grep 10.20.0.10 || true"
ssh vova@srv2.example.internal "sudo ip a show dev wg0 | grep 10.20.0.10 || true"
ssh vova@srv3.example.internal "sudo ip a show dev wg0 | grep 10.20.0.10 || true"
```

Expected: VIP appears on exactly one node.

## 4) MinIO health (x3)

```bash
ssh vova@srv1.example.internal 'curl -fsS http://10.20.0.1:9000/minio/health/live'
ssh vova@srv2.example.internal 'curl -fsS http://10.20.0.2:9000/minio/health/live'
ssh vova@srv3.example.internal 'curl -fsS http://10.20.0.3:9000/minio/health/live'
```

Expected: all 3 endpoints return success.

## 5) Monitoring timer (srv1)

```bash
ssh vova@srv1.example.internal 'systemctl is-active team-cli-json-monitor.timer'
ssh vova@srv1.example.internal 'sudo journalctl -u team-cli-json-monitor.service -n 20 --no-pager'
```

Expected: timer is `active`; no fresh `ALERT` lines unless a real incident exists.

## 6) Failover smoke (optional, +30s)
1. Stop keepalived on current VIP owner:
```bash
sudo systemctl stop keepalived
```
2. Check VIP moved to another node:
```bash
sudo ip a show dev wg0 | grep 10.20.0.10 || true
```
3. Start keepalived back:
```bash
sudo systemctl start keepalived
```

Expected: VIP migrates and returns after recovery.

