# Quickstart Runbook: 3 Nodes (Debian 13)

This runbook gives exact command order for `node-1`, `node-2`, `node-3`.

## 0) Prerequisites
- 3 Debian 13 VPS with SSH access.
- Nodes can reach each other in private network (Tailscale/ZeroTier recommended).
- Repo: `https://github.com/flakeman/team-cli-tracker`

## 1) On each node: clone and bootstrap
```bash
git clone https://github.com/flakeman/team-cli-tracker.git
cd team-cli-tracker
```

## 2) Node-specific config

### node-1
```bash
cp deploy/cluster.node-1.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

### node-2
```bash
cp deploy/cluster.node-2.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

### node-3
```bash
cp deploy/cluster.node-3.env.example deploy/cluster.env
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

## 3) Install and start systemd service
Run on each node:
```bash
sudo bash /opt/team-cli-tracker/deploy/install-systemd-service.sh /opt/team-cli-tracker/deploy/cluster.env
```

## 4) Verify
Run on each node:
```bash
systemctl is-active team-cli-tracker
sudo journalctl -u team-cli-tracker -n 50 --no-pager
```

## 5) Update rollout
On each node:
```bash
cd /opt/team-cli-tracker
git fetch origin
git checkout main
git pull --ff-only origin main
sudo systemctl restart team-cli-tracker
```

## 6) Rollback (single-node quick rollback)
```bash
cd /opt/team-cli-tracker
git log --oneline -n 5
git checkout <previous_commit_sha>
sudo systemctl restart team-cli-tracker
```

## 7) If Team Size Is 2 Or More Than 3

### Team size = 2
- Recommended: keep `3` voting nodes and add one lightweight witness node.
- Why: Raft quorum with 2 nodes is fragile (no fault tolerance, split-brain risk).

### Team size > 3
- Users do not need to be voting nodes.
- Recommended:
1. Keep `3` voting nodes for small/medium load.
2. Move to `5` voting nodes for higher load/availability.
3. Keep remaining participants as client/non-voting nodes.

### Quorum rule
- Use odd number of voting nodes: `3`, `5`, `7`.
- Quorum is `N/2 + 1`.

### 5-node mode
- Use templates:
1. `deploy/cluster.node-1.env.example`
2. `deploy/cluster.node-2.env.example`
3. `deploy/cluster.node-3.env.example`
4. `deploy/cluster.node-4.env.example`
5. `deploy/cluster.node-5.env.example`
