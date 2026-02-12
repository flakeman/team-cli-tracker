#!/usr/bin/env bash
set -euo pipefail

# Minimal PKI automation for test clusters:
# - init-ca
# - issue-node <node-id> <dns-or-ip>
# - revoke-node <node-id>

PKI_DIR="${PKI_DIR:-./deploy/pki}"
CA_KEY="$PKI_DIR/ca.key.pem"
CA_CRT="$PKI_DIR/ca.crt.pem"
CA_SRL="$PKI_DIR/ca.srl"

usage() {
  echo "usage:"
  echo "  PKI_DIR=./deploy/pki bash deploy/pki-manage.sh init-ca"
  echo "  PKI_DIR=./deploy/pki bash deploy/pki-manage.sh issue-node node-1 node1.local"
  echo "  PKI_DIR=./deploy/pki bash deploy/pki-manage.sh revoke-node node-1"
}

require_ca() {
  if [[ ! -f "$CA_KEY" || ! -f "$CA_CRT" ]]; then
    echo "missing CA files, run init-ca first"
    exit 1
  fi
}

mkdir -p "$PKI_DIR"

cmd="${1:-}"
case "$cmd" in
  init-ca)
    openssl genrsa -out "$CA_KEY" 4096
    openssl req -x509 -new -nodes -key "$CA_KEY" -sha256 -days 3650 \
      -subj "/CN=team-cli-tracker-test-ca" -out "$CA_CRT"
    echo "ok: CA created in $PKI_DIR"
    ;;

  issue-node)
    require_ca
    node_id="${2:-}"
    node_name="${3:-}"
    if [[ -z "$node_id" || -z "$node_name" ]]; then
      usage; exit 1
    fi
    node_key="$PKI_DIR/${node_id}.key.pem"
    node_csr="$PKI_DIR/${node_id}.csr.pem"
    node_crt="$PKI_DIR/${node_id}.crt.pem"
    ext="$PKI_DIR/${node_id}.ext.cnf"
    cat >"$ext" <<EOF
subjectAltName=DNS:${node_name}
extendedKeyUsage=serverAuth,clientAuth
EOF
    openssl genrsa -out "$node_key" 2048
    openssl req -new -key "$node_key" -subj "/CN=${node_id}" -out "$node_csr"
    openssl x509 -req -in "$node_csr" -CA "$CA_CRT" -CAkey "$CA_KEY" -CAcreateserial \
      -out "$node_crt" -days 365 -sha256 -extfile "$ext"
    rm -f "$node_csr" "$ext"
    echo "ok: issued cert for $node_id -> $node_crt"
    ;;

  revoke-node)
    require_ca
    node_id="${2:-}"
    if [[ -z "$node_id" ]]; then
      usage; exit 1
    fi
    node_crt="$PKI_DIR/${node_id}.crt.pem"
    if [[ ! -f "$node_crt" ]]; then
      echo "cert not found: $node_crt"
      exit 1
    fi
    idx="$PKI_DIR/index.txt"
    serial="$PKI_DIR/serial"
    crl="$PKI_DIR/ca.crl.pem"
    conf="$PKI_DIR/openssl-ca.cnf"
    touch "$idx"
    [[ -f "$serial" ]] || echo "1000" > "$serial"
    cat >"$conf" <<EOF
[ ca ]
default_ca = CA_default

[ CA_default ]
database = $idx
serial = $serial
default_md = sha256
policy = policy_any
private_key = $CA_KEY
certificate = $CA_CRT
default_crl_days = 30

[ policy_any ]
commonName = supplied
EOF
    openssl ca -config "$conf" -revoke "$node_crt" -batch
    openssl ca -config "$conf" -gencrl -out "$crl" -batch
    echo "ok: revoked $node_id, CRL: $crl"
    ;;

  *)
    usage
    exit 1
    ;;
esac
