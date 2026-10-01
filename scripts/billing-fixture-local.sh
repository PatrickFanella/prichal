#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
node scripts/init-billing-fixture.mjs
case "${1:-help}" in
 run) exec node deploy/local/fixtures/billing.mjs ;;
 api|worker|bind)
  export PDS_BILLING_MODE=fixture
  export PDS_BILLING_FIXTURE_URL=http://127.0.0.1:5560
  export PDS_BILLING_WEBHOOK_KEY_FILE="$PWD/.local/billing-fixture/webhook.key"
  export PDS_BILLING_FIXTURE_KEY_FILE="$PWD/.local/billing-fixture/api.key"
  action=$1;shift
  if [[ $action == bind ]]; then exec ./scripts/dev.sh billing-fixture-admin "$@"; fi
  exec ./scripts/dev.sh "$action" "$@" ;;
 *) printf 'Usage: scripts/billing-fixture-local.sh {run|api|worker|bind}\n' ;;
esac
