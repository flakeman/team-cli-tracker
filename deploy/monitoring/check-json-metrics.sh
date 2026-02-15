#!/usr/bin/env bash
set -euo pipefail

TARGETS_CSV="${1:-srv1.example.internal:4101,srv2.example.internal:4101,srv3.example.internal:4101}"
IFS=',' read -r -a TARGETS <<< "${TARGETS_CSV}"

has_err=0
for t in "${TARGETS[@]}"; do
  target="$(echo "$t" | xargs)"
  [[ -z "${target}" ]] && continue
  health="$(curl -fsS --max-time 3 "http://${target}/api/v1/healthz" || true)"
  if [[ "${health}" != *"\"status\":\"ok\""* ]]; then
    echo "ALERT healthz failed target=${target}"
    has_err=1
    continue
  fi
  metrics_json="$(curl -fsS --max-time 3 "http://${target}/api/v1/metrics" || true)"
  if [[ -z "${metrics_json}" ]]; then
    echo "ALERT metrics empty target=${target}"
    has_err=1
    continue
  fi
  parse_out="$(python3 -c "import json,sys; m=json.loads(sys.stdin.read()); print(int(m.get('pull_errors',0))); print(int(m.get('authz_denied_total',0))); print(int(m.get('attachment_verify_mismatch',0)))" <<< "${metrics_json}" 2>/dev/null || true)"
  if [[ -z "${parse_out}" ]]; then
    echo "ALERT metrics parse failed target=${target}"
    has_err=1
    continue
  fi
  pull_errors="$(echo "${parse_out}" | sed -n '1p')"
  authz_denied="$(echo "${parse_out}" | sed -n '2p')"
  mismatch="$(echo "${parse_out}" | sed -n '3p')"
  if [[ "${pull_errors}" -gt 0 ]]; then
    echo "ALERT pull_errors=${pull_errors} target=${target}"
    has_err=1
  fi
  if [[ "${mismatch}" -gt 0 ]]; then
    echo "ALERT attachment_verify_mismatch=${mismatch} target=${target}"
    has_err=1
  fi
  echo "OK target=${target} authz_denied_total=${authz_denied}"
done

exit "${has_err}"


