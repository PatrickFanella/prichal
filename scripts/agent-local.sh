#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
agent_root="$PWD/.local/node-agent"
mkdir -p "$agent_root/certs" "$agent_root/state"
chmod 700 "$agent_root" "$agent_root/certs" "$agent_root/state"
agent_certs="$agent_root/certs"
case "${1:-help}" in
  init)
    if [[ -e "$agent_certs/ca.crt" ]]; then
      for name in ca.key agent.crt agent.key worker.crt worker.key; do
        [[ -f "$agent_certs/$name" ]] || { printf 'Incomplete local agent PKI. Preserve and inspect %s\n' "$agent_certs" >&2; exit 1; }
      done
      printf 'Existing local agent PKI retained.\n'
      exit 0
    fi
    umask 077
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 7 -subj '/CN=pds-ass-local-ca' -keyout "$agent_certs/ca.key" -out "$agent_certs/ca.crt" > "$agent_root/pki.log" 2>&1
    for role in agent worker; do
      openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -subj "/CN=pds-ass-$role" -keyout "$agent_certs/$role.key" -out "$agent_certs/$role.csr" >> "$agent_root/pki.log" 2>&1
      if [[ "$role" == agent ]]; then
        printf 'subjectAltName=IP:127.0.0.1,DNS:localhost\nextendedKeyUsage=serverAuth\n' > "$agent_certs/extensions.cnf"
      else
        printf 'extendedKeyUsage=clientAuth\n' > "$agent_certs/extensions.cnf"
      fi
      openssl x509 -req -in "$agent_certs/$role.csr" -CA "$agent_certs/ca.crt" -CAkey "$agent_certs/ca.key" -CAcreateserial -days 7 -extfile "$agent_certs/extensions.cnf" -out "$agent_certs/$role.crt" >> "$agent_root/pki.log" 2>&1
    done
    printf 'Local seven-day agent PKI created in %s\n' "$agent_certs"
    ;;
  run)
    export PDS_MODE=local PDS_AGENT_CA_FILE="$agent_certs/ca.crt" PDS_AGENT_CERT_FILE="$agent_certs/agent.crt" PDS_AGENT_KEY_FILE="$agent_certs/agent.key" PDS_AGENT_STATE_DIRECTORY="$agent_root/state"
    agent_socket_url=$(docker context inspect --format '{{.Endpoints.docker.Host}}')
    [[ "$agent_socket_url" == unix:///* ]] || { printf 'Local qualification needs a Unix Docker socket.\n' >&2; exit 1; }
    export PDS_AGENT_DOCKER_SOCKET="${agent_socket_url#unix://}"
    cd backend
    exec go run ./cmd/node-agent
    ;;
  ensure)
    instance_id="${2:-}"
    [[ "$instance_id" =~ ^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$ ]] || { printf 'Supply a lowercase instance UUID.\n' >&2; exit 2; }
    curl --silent --show-error --fail --max-time 30 --cacert "$agent_certs/ca.crt" --cert "$agent_certs/worker.crt" --key "$agent_certs/worker.key" -X PUT -H 'Content-Type: application/json' --data '{}' "https://127.0.0.1:9091/v1/instances/$instance_id"
    ;;
  *) printf 'Usage: scripts/agent-local.sh {init|run|ensure INSTANCE_UUID}\n'; exit 2 ;;
esac
