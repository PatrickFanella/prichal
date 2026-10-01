#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .local
if [[ ! -f .local/local.env ]]; then
  (umask 077; password=$(openssl rand -hex 24); printf 'PDS_LOCAL_DB_PASSWORD=%s\nPDS_DATABASE_URL=postgres://pds_local:%s@127.0.0.1:35433/pds_local?sslmode=disable\n' "$password" "$password" > .local/local.env)
fi
set -a
source .local/local.env
set +a
# Local-only defaults. Direct production startup has no fixture fallback.
unset PDS_OIDC_CLIENT_SECRET
node scripts/init-enrollment-key.mjs
node scripts/init-export-store.mjs
export PDS_LOCAL_EXPORT_DIRECTORY="$PWD/.local/exports"
export PDS_LOCAL_EXPORT_KEY_FILE="$PWD/.local/export.key"
export PDS_ENROLLMENT_KEY_FILE="$PWD/.local/enrollment.key"
export PDS_MODE=local
export PDS_OIDC_ISSUER=http://localhost:5556
export PDS_OIDC_CLIENT_ID=pds-local
export PDS_PUBLIC_ORIGIN=http://localhost:5173
case "${1:-help}" in
  db) docker compose up -d --wait db ;;
  issuer) cd backend; exec go run ./cmd/test-issuer ;;
  node-enroll) shift; cd backend; exec go run ./cmd/node-enroll "$@" ;;
  node-admin) shift; cd backend; exec go run ./cmd/node-admin "$@" ;;
  billing-fixture-admin) shift; cd backend; exec go run ./cmd/billing-fixture-admin "$@" ;;
  entitlement-admin) shift; cd backend; exec go run ./cmd/entitlement-admin "$@" ;;
  service-admin) shift; cd backend; exec go run ./cmd/service-admin "$@" ;;
  enrollment-admin) shift; cd backend; exec go run ./cmd/enrollment-admin "$@" ;;
  owner-admin) shift; cd backend; exec go run ./cmd/owner-admin "$@" ;;
  deployment-admin) shift; cd backend; exec go run ./cmd/deployment-admin "$@" ;;
  worker) cd backend; exec go run ./cmd/worker ;;
  api) cd backend; exec go run ./cmd/api ;;
  web) cd web; exec pnpm dev ;;
  smoke) exec ./scripts/smoke.sh ;;
  down) docker compose down ;;
  *) printf 'Usage: scripts/dev.sh {db|issuer|api|worker|billing-fixture-admin|entitlement-admin|service-admin|owner-admin|deployment-admin|enrollment-admin|node-admin|node-enroll|web|smoke|down}\n'; exit 2 ;;
esac
