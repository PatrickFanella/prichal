#!/usr/bin/env bash
# Workstation-only PDS qualification. Isolated from public identity infrastructure.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .local
if [[ ! -f .local/pds.env ]]; then
  (umask 077; node --input-type=module > .local/pds.env <<'JS'
import { generateKeyPairSync, randomBytes } from 'node:crypto';
const { privateKey } = generateKeyPairSync('ec', { namedCurve: 'secp256k1' });
const jwk = privateKey.export({ format: 'jwk' });
console.log(`PDS_PLC_ROTATION_KEY_K256_PRIVATE_KEY_HEX=${Buffer.from(jwk.d,'base64url').toString('hex')}`);
console.log(`PDS_JWT_SECRET=${randomBytes(32).toString('hex')}`);
console.log(`PDS_ADMIN_PASSWORD=${randomBytes(32).toString('hex')}`);
JS
  )
fi
compose=(docker compose -f deploy/local/compose.yaml)
case "${1:-help}" in
  up) "${compose[@]}" up -d ;;
  check) node scripts/check-pds.mjs ;;
  qualify) node scripts/qualify-pds.mjs ;;
  qualify-account) node scripts/qualify-account.mjs ;;
  qualify-recovery) node scripts/qualify-recovery.mjs ;;
  restart) "${compose[@]}" restart pds ;;
  down) "${compose[@]}" down ;;
  *) printf 'Usage: scripts/pds-local.sh {up|check|qualify|qualify-account|qualify-recovery|restart|down}\n'; exit 2 ;;
esac
