# Deploy Toolkit

This folder contains helper scripts for a 3-node Debian 13 test cluster.

## Files
- `cluster.env.example` - common variables template.
- `cluster.node-1.env.example` - ready config template for node-1.
- `cluster.node-2.env.example` - ready config template for node-2.
- `cluster.node-3.env.example` - ready config template for node-3.
- `cluster.node-4.env.example` - ready config template for node-4 (5-node mode).
- `cluster.node-5.env.example` - ready config template for node-5 (5-node mode).
- `bootstrap-debian13.sh` - base OS hardening + Go install + repo clone/update.
- `gen-node-config.sh` - creates `configs/node-<id>.yaml` from env values.
- `install-systemd-service.sh` - installs and starts `team-cli-tracker.service`.
- `RUNBOOK-QUICKSTART-3NODES.md` - exact per-node command sequence.

## Typical Flow
1. Copy `cluster.env.example` to `cluster.env` and fill values.
2. Run `bootstrap-debian13.sh` on each VPS.
3. Run `gen-node-config.sh` on each VPS with node-specific IDs/ports/peers.
4. Run `install-systemd-service.sh` on each VPS.
5. Validate service and logs.
