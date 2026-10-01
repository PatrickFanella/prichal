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
case "${1:-help}" in
  db) docker compose up -d --wait db ;;
  api) cd backend; exec go run ./cmd/api ;;
  web) cd web; exec pnpm dev ;;
  smoke) exec ./scripts/smoke.sh ;;
  down) docker compose down ;;
  *) printf 'Usage: scripts/dev.sh {db|api|web|smoke|down}\n'; exit 2 ;;
esac
