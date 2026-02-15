# Isolated Deployment: WireGuard + Multi-Gateway + Single Entrypoint

## Target
- No public node exposure.
- All nodes in private WireGuard network.
- Single client entrypoint: `board.internal`.
- Gateway runs on all nodes for higher stability.

## VPN Addressing
- `srv1` = `10.20.0.1`
- `srv2` = `10.20.0.2`
- `srv3` = `10.20.0.3`
- next nodes continue sequence (`10.20.0.4`, ...)

## Required Servers
- Minimum: 3 servers (`srv1/srv2/srv3`).
- No external SaaS registration required.
- Internal DNS is required (`board.internal` record management).

---

## Step 1: WireGuard on all nodes

On each node:
1. `cp deploy/wireguard.env.example deploy/wireguard.env`
2. Fill:
   - `WG_NODE_ID`
   - `WG_ADDRESS_CIDR`
   - `WG_PRIVATE_KEY`
   - `WG_PEERS`
3. Install:
```bash
sudo bash deploy/install-wireguard.sh deploy/wireguard.env
```

Check:
```bash
sudo wg show
ip -4 addr show wg0
```

---

## Step 2: Gateway on all nodes

On each node:
1. `cp deploy/gateway.env.example deploy/gateway.env`
2. Set:
   - `GATEWAY_MODE=public_proxy`
   - `BOARD_DOMAIN=board.internal`
   - `GATEWAY_ENABLE_TLS=false` (or true if internal PKI is ready)
   - `GATEWAY_BIND_ADDRESS=<node vpn ip>` (e.g. `10.20.0.1`)
   - `GATEWAY_LISTEN_PORT=8080`
   - `GATEWAY_UPSTREAMS=http://10.20.0.1:4101,http://10.20.0.2:4101,http://10.20.0.3:4101`
3. Install:
```bash
sudo bash deploy/install-gateway-caddy.sh deploy/gateway.env
```

---

## Step 3: Single Entrypoint (`board.internal`) options

Choose one option.

### Option A: Internal L4/L7 Load Balancer
- Put LB in front of `10.20.0.1:8080`, `10.20.0.2:8080`, `10.20.0.3:8080`.
- DNS: `board.internal -> <LB VIP>`.
- Best operational control.

### Option B: VRRP/keepalived (VIP)
- Run keepalived on all three nodes.
- One virtual IP (example `10.20.0.10`) floats between nodes.
- DNS: `board.internal -> 10.20.0.10`.
- Good when dedicated LB is unavailable.

### Option C: DNS multi-A + health check
- DNS:
  - `board.internal -> 10.20.0.1`
  - `board.internal -> 10.20.0.2`
  - `board.internal -> 10.20.0.3`
- DNS server removes dead targets by health-check.
- Simplest infra if DNS supports checks.

---

## Failure Behavior
- If one gateway node dies:
  - Option A/B/C keeps entrypoint available when configured correctly.
- If one tracker node dies:
  - 3-node cluster continues with quorum.
- If two nodes die:
  - write-path may stop due to quorum loss.

---

## Client Connection

Windows:
```powershell
.\node.exe board --project-id OPS --peers http://board.internal:8080 --peer-token <token> --refresh 2s --view all
```

Linux:
```bash
./node board --project-id OPS --peers http://board.internal:8080 --peer-token <token> --refresh 2s --view all
```
