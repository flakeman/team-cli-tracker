# Deployment: Ubuntu 24, 3 VPS, Real Users

## RU

Р­С‚РѕС‚ runbook РѕРїРёСЃС‹РІР°РµС‚ production-like СЂР°Р·РІС‘СЂС‚С‹РІР°РЅРёРµ `team-cli-tracker` РЅР° 3 VPS Рё РїРѕРґРєР»СЋС‡РµРЅРёРµ СЂРµР°Р»СЊРЅС‹С… РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№ С‡РµСЂРµР· issuer/policy.

## 0) РўРµСЂРјРёРЅС‹
- VPS/Node: СЃРµСЂРІРµСЂРЅС‹Р№ СѓР·РµР» РєР»Р°СЃС‚РµСЂР° (РІ РІР°С€РµРј СЃР»СѓС‡Р°Рµ 3).
- Real user: С‡РµР»РѕРІРµРє СЃ РѕС‚РґРµР»СЊРЅС‹Рј `user_id`, СЂРѕР»СЊСЋ Рё token.
- Simulated actor: С‚РµС…РЅРёС‡РµСЃРєРёР№ Р°РєС‚РѕСЂ РІ С‚РµСЃС‚Рµ (РЅРµ СЂРµР°Р»СЊРЅС‹Р№ С‡РµР»РѕРІРµРє).

РљРѕР»РёС‡РµСЃС‚РІРѕ VPS РЅРµ РѕРіСЂР°РЅРёС‡РёРІР°РµС‚ РєРѕР»РёС‡РµСЃС‚РІРѕ РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№.

## 1) РџСЂРµРґРїРѕСЃС‹Р»РєРё
- Ubuntu 24 РЅР° 3 VPS.
- SSH endpoints:
  - `srv1-22221 = srv1.example.internal` (SSH port `22`)
  - `srv2-22222 = srv2.example.internal` (SSH port `22`)
  - `srv3-22223 = srv3.example.internal` (SSH port `22`)
- РћС‚РєСЂС‹С‚С‹ СЃРµС‚РµРІС‹Рµ РїРѕСЂС‚С‹ РґР»СЏ node API/peer communication.
- Р Р°Р±РѕС‚Р°РµС‚ issuer/policy endpoint РґР»СЏ authn/authz.
- Р›РѕРєР°Р»СЊРЅР°СЏ РјР°С€РёРЅР° СЃ Go РґР»СЏ СЃР±РѕСЂРєРё Р±РёРЅР°СЂРЅРёРєР°.

## 2) РЎР±РѕСЂРєР° Р±РёРЅР°СЂРЅРёРєР° (Р»РѕРєР°Р»СЊРЅРѕ)
```bash
cd team-cli-tracker
go test ./...
GOOS=linux GOARCH=amd64 go build -o node ./cmd/node
```

## 3) Р”РѕСЃС‚Р°РІРєР° Р±РёРЅР°СЂРЅРёРєР° РЅР° РІСЃРµ VPS
РџСЂРёРјРµСЂ РґР»СЏ С‚РµРєСѓС‰РёС… С…РѕСЃС‚РѕРІ:
```bash
scp ./node vova@srv1.example.internal:/home/vova/tct/node
scp ./node vova@srv2.example.internal:/home/vova/tct/node
scp ./node vova@srv3.example.internal:/home/vova/tct/node
```

РќР° РєР°Р¶РґРѕРј VPS:
```bash
chmod +x ~/tct/node
mkdir -p ~/tct/data ~/tct/log
```

## 4) Р‘С‹СЃС‚СЂС‹Р№ Р·Р°РїСѓСЃРє РІ screen (РѕРїРµСЂР°С†РёРѕРЅРЅС‹Р№ smoke)
РќР° РєР°Р¶РґРѕРј VPS:
```bash
screen -S tct_node -dm bash -lc '~/tct/node serve --project-id OPS --listen :4101 --data-dir ~/tct/data --secure-mode-required=true >> ~/tct/log/node.log 2>&1'
screen -ls
```

РџСЂРѕРІРµСЂРєР°:
```bash
tail -n 50 ~/tct/log/node.log
```

РћСЃС‚Р°РЅРѕРІРёС‚СЊ:
```bash
screen -S tct_node -X quit
```

## 5) РЈСЃС‚РѕР№С‡РёРІС‹Р№ Р·Р°РїСѓСЃРє С‡РµСЂРµР· systemd (СЂРµРєРѕРјРµРЅРґСѓРµС‚СЃСЏ)
`/etc/systemd/system/team-cli-tracker.service`:
```ini
[Unit]
Description=team-cli-tracker node
After=network-online.target
Wants=network-online.target

[Service]
User=vova
WorkingDirectory=/home/vova/tct
ExecStart=/home/vova/tct/node serve --project-id OPS --listen :4101 --data-dir /home/vova/tct/data --secure-mode-required=true
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

РџСЂРёРјРµРЅРёС‚СЊ:
```bash
sudo systemctl daemon-reload
sudo systemctl enable team-cli-tracker
sudo systemctl restart team-cli-tracker
sudo systemctl status team-cli-tracker --no-pager
```

Р›РѕРіРё:
```bash
journalctl -u team-cli-tracker -f
```

## 6) РћРЅР±РѕСЂРґРёРЅРі СЂРµР°Р»СЊРЅС‹С… РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№
Р”Р»СЏ РєР°Р¶РґРѕРіРѕ С‡РµР»РѕРІРµРєР°:
1. РЎРѕР·РґР°С‚СЊ identity РІ issuer (`user_id`).
2. РќР°Р·РЅР°С‡РёС‚СЊ СЂРѕР»СЊ (`admin`, `lead`, `dev`, `qa`, `viewer`).
3. Р’С‹РїСѓСЃС‚РёС‚СЊ token СЃ РѕРіСЂР°РЅРёС‡РµРЅРЅС‹Рј TTL.
4. РџРµСЂРµРґР°С‚СЊ token Р±РµР·РѕРїР°СЃРЅРѕ (СЃРµРєСЂРµС‚-С…СЂР°РЅРёР»РёС‰Рµ, РЅРµ chat/shell history).

Р РµРєРѕРјРµРЅРґСѓРµРјР°СЏ РјРёРЅРёРјР°Р»СЊРЅР°СЏ РјР°С‚СЂРёС†Р°:
- `admin1` -> `admin`
- `lead1` -> `lead`
- `dev1` -> `dev`
- `qa1` -> `qa`
- `viewer1` -> `viewer`

## 7) РџРѕРґРєР»СЋС‡РµРЅРёРµ СЂРµР°Р»СЊРЅРѕРіРѕ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ Рє РґРѕСЃРєРµ
Live mode:
```bash
team-cli-tracker board watch \
  --server http://<node-host>:4101 \
  --project-id OPS \
  --user-id <user_id> \
  --auth-token <token> \
  --view all
```

Interactive mode:
```bash
team-cli-tracker board watch \
  --server http://<node-host>:4101 \
  --project-id OPS \
  --user-id <user_id> \
  --auth-token <token> \
  --interactive \
  --interactive-refresh 2s
```

РљРѕРјР°РЅРґС‹ РІРЅСѓС‚СЂРё interactive:
- `help`
- `create`
- `move`
- `comment`
- `view`
- `counts`
- `table`
- `pause`
- `resume`
- `quit`

## 8) РџСЂРѕРІРµСЂРєР° РїСЂР°РІ (allow/deny)
РџРѕР·РёС‚РёРІРЅС‹Рµ РєРµР№СЃС‹:
- `lead/dev` РјРѕРіСѓС‚ СЃРѕР·РґР°РІР°С‚СЊ Рё РґРІРёРіР°С‚СЊ Р·Р°РґР°С‡Рё РІ РїСЂРµРґРµР»Р°С… policy.
- `qa` РјРѕР¶РµС‚ РїРµСЂРµРІРѕРґРёС‚СЊ Р·Р°РґР°С‡Рё РїРѕ СЂР°Р·СЂРµС€С‘РЅРЅС‹Рј РїРµСЂРµС…РѕРґР°Рј.

РќРµРіР°С‚РёРІРЅС‹Рµ РєРµР№СЃС‹:
- `viewer` РЅРµ РјРѕР¶РµС‚ `create/move/comment`.
- РџРѕР»СЊР·РѕРІР°С‚РµР»СЊ Р±РµР· СЂРѕР»Рё РЅРµ РјРѕР¶РµС‚ РјСѓС‚РёСЂРѕРІР°С‚СЊ РґР°РЅРЅС‹Рµ.

## 9) РђСѓРґРёС‚-РґРѕРєР°Р·Р°С‚РµР»СЊСЃС‚РІРѕ СЂРµР°Р»СЊРЅС‹С… РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№
РџРѕСЃР»Рµ С‚РµСЃС‚Р°:
```bash
team-cli-tracker audit export --all --format jsonl
team-cli-tracker audit export --from 2026-02-13T00:00:00Z --to 2026-02-13T23:59:59Z --user admin1,lead1,dev1,qa1,viewer1 --format csv
team-cli-tracker audit verify-integrity
```

РљСЂРёС‚РµСЂРёР№:
- Р’ СЌРєСЃРїРѕСЂС‚Рµ РІРёРґРЅС‹ РґРµР№СЃС‚РІРёСЏ СЃ СЂРµР°Р»СЊРЅС‹РјРё `user_id`.
- РќРµС‚ Р»РѕР¶РЅС‹С… "СѓСЃРїРµС€РЅС‹С…" РјСѓС‚Р°С†РёР№ Сѓ Р·Р°РїСЂРµС‰С‘РЅРЅС‹С… СЂРѕР»РµР№.
- Integrity check РїСЂРѕС…РѕРґРёС‚.

## 10) РћРїРµСЂР°С†РёРѕРЅРЅС‹Р№ С‡РµРєР»РёСЃС‚
- `secure-mode-required=true` РІ production.
- UTC РЅР° РІСЃРµС… VPS.
- Р РѕС‚Р°С†РёСЏ С‚РѕРєРµРЅРѕРІ Рё СЃРµРєСЂРµС‚РѕРІ.
- Р РµРіСѓР»СЏСЂРЅС‹Р№ СЌРєСЃРїРѕСЂС‚ Р°СѓРґРёС‚Р°.
- Р’ smoke-РѕС‚С‡С‘С‚Рµ РІСЃРµРіРґР° СЂР°Р·РґРµР»СЏС‚СЊ:
  - simulated actors,
  - real users (token-based).

