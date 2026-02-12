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
cp deploy/cluster.env.example deploy/cluster.env
```

Edit `deploy/cluster.env` on each node:
- `NODE_ID` must be unique (`node-1`, `node-2`, `node-3`)
- `NODE_LISTEN_ADDR` must match the local port
- `NODE_PEERS` must list the other two nodes

Run bootstrap:
```bash
sudo bash deploy/bootstrap-debian13.sh deploy/cluster.env
```

## 2) Node-specific config

### node-1
Set in `deploy/cluster.env`:
```bash
NODE_ID="node-1"
NODE_LISTEN_ADDR="/ip4/0.0.0.0/tcp/4101"
NODE_PEERS="node-2,node-3"
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

### node-2
Set in `deploy/cluster.env`:
```bash
NODE_ID="node-2"
NODE_LISTEN_ADDR="/ip4/0.0.0.0/tcp/4102"
NODE_PEERS="node-1,node-3"
```
Generate config:
```bash
sudo -u teamtracker bash /opt/team-cli-tracker/deploy/gen-node-config.sh /opt/team-cli-tracker/deploy/cluster.env
```

### node-3
Set in `deploy/cluster.env`:
```bash
NODE_ID="node-3"
NODE_LISTEN_ADDR="/ip4/0.0.0.0/tcp/4103"
NODE_PEERS="node-1,node-2"
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

