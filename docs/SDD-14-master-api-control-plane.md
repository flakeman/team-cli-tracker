# SDD-14: Master API Control Plane

## Status
- Proposed
- Date: 2026-02-15
- Revision: 1

## Context
РўРµРєСѓС‰РёР№ API РїРѕРєСЂС‹РІР°РµС‚ Р±РѕР»СЊС€СѓСЋ С‡Р°СЃС‚СЊ РґРѕРјРµРЅРЅС‹С… РѕРїРµСЂР°С†РёР№, РЅРѕ РѕС‚СЃСѓС‚СЃС‚РІСѓРµС‚ РµРґРёРЅС‹Р№ control-plane СЃР»РѕР№ РґР»СЏ С†РµРЅС‚СЂР°Р»РёР·РѕРІР°РЅРЅРѕРіРѕ СѓРїСЂР°РІР»РµРЅРёСЏ СЃРёСЃС‚РµРјРѕР№ "РёР· РѕРґРЅРѕР№ С‚РѕС‡РєРё".

## Goals
1. Р’РІРµСЃС‚Рё `Master API` РєР°Рє РµРґРёРЅС‹Р№ СѓРїСЂР°РІР»РµРЅС‡РµСЃРєРёР№ СЃР»РѕР№.
2. Р”Р°С‚СЊ РєРѕРЅСЃРёСЃС‚РµРЅС‚РЅС‹Р№ namespace РґР»СЏ orchestration РѕРїРµСЂР°С†РёР№.
3. РћР±РµСЃРїРµС‡РёС‚СЊ РїРѕР»РЅСѓСЋ parity СЃ CLI Рё СЃСѓС‰РµСЃС‚РІСѓСЋС‰РёРјРё API РїСѓС‚СЏРјРё.
4. РЎРґРµР»Р°С‚СЊ Р±РµР·РѕРїР°СЃРЅС‹Р№ bootstrap РґР»СЏ РІРЅРµС€РЅРёС… РёРЅС‚РµРіСЂР°С†РёР№.

## Non-Goals
1. РџРѕР»РЅР°СЏ Р·Р°РјРµРЅР° РІСЃРµС… СЃСѓС‰РµСЃС‚РІСѓСЋС‰РёС… endpoint РІ РѕРґРёРЅ СЂРµР»РёР·.
2. РњРёРіСЂР°С†РёСЏ РєР»РёРµРЅС‚РѕРІ Р±РµР· backward compatibility.

## API Scope (v1)
1. Bootstrap:
- `GET /api/v1/master/health`
- `GET /api/v1/master/capabilities`

2. Control namespaces (phase rollout):
- `master/issues/*`
- `master/attachments/*`
- `master/team/*`
- `master/auth/*`
- `master/trust/*`
- `master/governance/*`
- `master/audit/*`
- `master/webhooks/*`

3. Cluster admin (phase rollout):
- `master/cluster/nodes`
- `master/cluster/health`
- `master/cluster/reconfigure`

## Security
1. RBAC РјРёРЅРёРјСѓРј: `admin`, `lead` РґР»СЏ control-plane.
2. Mutating and security-sensitive master calls must be written to audit; read-only health/capability calls may be excluded.
3. РРґРµРјРїРѕС‚РµРЅС‚РЅРѕСЃС‚СЊ РјСѓС‚Р°С†РёР№ С‡РµСЂРµР· request-id (phase rollout).

## Acceptance Criteria
1. Bootstrap endpoints РґРѕСЃС‚СѓРїРЅС‹ Рё РІРѕР·РІСЂР°С‰Р°СЋС‚ capability map.
2. Р”РѕРєСѓРјРµРЅС‚Р°С†РёСЏ СЃРѕРґРµСЂР¶РёС‚ roadmap РјРёРіСЂР°С†РёРё РІ master namespace.
3. РћРїСЂРµРґРµР»РµРЅС‹ СЌС‚Р°РїС‹ РїРµСЂРµС…РѕРґР° Р±РµР· breaking changes.
