# Deployment: Single Entrypoint for Multi-Node Cluster

## Goal
Provide one stable URL for users, while backend has multiple tracker nodes.

Client always connects to one endpoint:
- `https://board.example.com` (recommended)
- or `http://board.example.com:8080` (lab/private)

## Topology
- Tracker nodes: `srv1/srv2/srv3` (`:4101`) in private/overlay network.
- Gateway node: reverse proxy (Caddy) with upstreams to all nodes.

---

## Scenario A: All Machines Have Dynamic Public IP

Use overlay network + tunnel:
1. Join all tracker nodes and gateway node to Tailscale/ZeroTier.
2. Configure tracker peers via overlay addresses.
3. Install Caddy gateway on gateway node:
   - `deploy/install-gateway-caddy.sh`
4. Publish gateway through Cloudflare Tunnel:
   - `deploy/install-gateway-cloudflared.sh`
5. Users connect to one DNS name:
   - `https://board.example.com`

Why this works:
- dynamic ISP IP does not matter;
- tunnel gives stable public hostname;
- internal cluster traffic stays in overlay network.

---

## Scenario B: Mixed Dynamic/Static IP

Use node with static IP as gateway:
1. Pick static-IP host as gateway.
2. Join all nodes to overlay network (recommended).
3. Install Caddy gateway on static-IP host:
   - `deploy/install-gateway-caddy.sh`
4. Point DNS A/AAAA of `board.example.com` to gateway static IP.
5. Users connect only to:
   - `https://board.example.com`

Optional:
- If static-IP host is unavailable, run backup gateway and switch DNS/LB target.

---

## Quickstart (Gateway Host)

1. Prepare env:
```bash
cd team-cli-tracker
cp deploy/gateway.env.example deploy/gateway.env
```

2. Edit `deploy/gateway.env`:
- `BOARD_DOMAIN`
- `GATEWAY_UPSTREAMS`
- `GATEWAY_MODE`
- `GATEWAY_ENABLE_TLS`

3. Install reverse proxy:
```bash
sudo bash deploy/install-gateway-caddy.sh deploy/gateway.env
```

4. (Dynamic IP + Tunnel mode only):
```bash
sudo bash deploy/install-gateway-cloudflared.sh deploy/gateway.env
```

5. Verify:
```bash
curl -I https://board.example.com
```

---

## Client Connection

Windows PowerShell:
```powershell
.\node.exe board --project-id OPS --peers https://board.example.com --peer-token <token> --refresh 2s --view all
```

Linux:
```bash
./node board --project-id OPS --peers https://board.example.com --peer-token <token> --refresh 2s --view all
```

---

## Security Notes
- Prefer TLS endpoint (`https://`).
- Keep tracker node ports private; expose only gateway.
- Use token auth for client API/board access.
- Restrict SSH to key-based auth where possible.
