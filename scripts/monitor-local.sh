#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f .local/local.env ]] || { printf 'Start the local database first.\n' >&2; exit 1; }
set -a
source .local/local.env
set +a
export PDS_MODE=local
export PDS_WORKER_AGENT_CA_FILE="$PWD/.local/node-agent/certs/ca.crt"
export PDS_WORKER_AGENT_CERT_FILE="$PWD/.local/node-agent/certs/worker.crt"
export PDS_WORKER_AGENT_KEY_FILE="$PWD/.local/node-agent/certs/worker.key"
mkdir -p .local/monitor
chmod 700 .local/monitor
(cd backend && go build -o ../.local/monitor/monitor ./cmd/monitor)
case "${1:-once}" in
 once) exec .local/monitor/monitor ;;
 watch)
  trap 'exit 0' INT TERM
  while true; do
   if ! .local/monitor/monitor; then printf 'Monitoring unavailable; prior findings remain retained.\n' >&2; fi
   sleep 30
  done
  ;;
 *) printf 'Usage: scripts/monitor-local.sh {once|watch}\n' >&2; exit 2 ;;
esac
