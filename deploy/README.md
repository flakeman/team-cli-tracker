# Deploy Toolkit

This folder contains helper scripts for a 3-node Debian 13 test cluster.

## Files
- `cluster.env.example` - common variables template.
- `bootstrap-debian13.sh` - base OS hardening + Go install + repo clone/update.
- `gen-node-config.sh` - creates `configs/node-<id>.yaml` from env values.
- `install-systemd-service.sh` - installs and starts `team-cli-tracker.service`.

## Typical Flow
1. Copy `cluster.env.example` to `cluster.env` and fill values.
2. Run `bootstrap-debian13.sh` on each VPS.
3. Run `gen-node-config.sh` on each VPS with node-specific IDs/ports/peers.
4. Run `install-systemd-service.sh` on each VPS.
5. Validate service and logs.

