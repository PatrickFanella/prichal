# Local node enrollment and placement

Node enrollment is a separate private HTTPS service. It holds the dedicated CA signer; the public management API and node agent do not load the CA private key. The local commands require `PDS_MODE=local`, and node administration refuses any database other than `pds_local`.

Initialize the local agent PKI first with `./scripts/agent-local.sh init`. In a separate terminal:

```sh
PDS_ENROLLMENT_CA_FILE="$PWD/.local/node-agent/certs/ca.crt" \
PDS_ENROLLMENT_CA_KEY_FILE="$PWD/.local/node-agent/certs/ca.key" \
PDS_ENROLLMENT_CERT_FILE="$PWD/.local/node-agent/certs/agent.crt" \
PDS_ENROLLMENT_KEY_FILE="$PWD/.local/node-agent/certs/agent.key" \
./scripts/dev.sh node-admin serve
```

Create a one-time bootstrap file for a private local endpoint. Paths passed through dev.sh must be absolute because it changes directory to backend:

```sh
./scripts/dev.sh node-admin issue \
  --endpoint https://127.0.0.1:9091 --slots 2 \
  --out "$PWD/.local/node-bootstrap.json"

PDS_ENROLLMENT_CA_FILE="$PWD/.local/node-agent/certs/ca.crt" \
./scripts/dev.sh node-enroll \
  --bootstrap "$PWD/.local/node-bootstrap.json" \
  --out "$PWD/.local/node-agent/enrolled"
```

Restart `./scripts/agent-local.sh run` after enrollment. It selects the enrolled certificate and private key when present. Node keys remain on the node; the registry stores a public-key hash and public certificate. The signer discards CSR-selected subject names and extensions, and issues only a node server certificate for the operator's registered endpoint. The certificate expires within 24 hours and never outlives its CA.

The token expires after 15 minutes. The database keeps only its hash and consumes it in the same transaction that activates the node and stores its public key. Unknown, expired and replayed credentials are rejected. An invalid CSR does not consume the credential. If the response is lost, the client retrieves the already-issued public certificate and verifies its node ID, CA and correspondence to the retained private key. It does not issue another identity.

Local placement reserves a slot in PostgreSQL while locking the deployment and candidate nodes. Each deployment has one durable placement. Draining nodes accept no new placements; existing reservations retain their node. An unavailable slot leaves an explicit capacity error and preserves all existing data. Slots are an operator ceiling, not measured hardware capacity. Automatic agent measurements and worker integration remain subsequent work.

Schema version 4 adds the node registry, bootstrap hashes and placement records. Restart the API after an operator command upgrades the local database. The required smoke gate verifies enrollment expiry/replay, CSR role restrictions, certificate trust and concurrent reservation without oversubscription.

This completes local protocol qualification only. Provider selection, cloud bootstrap, secret custody, production certificate rotation and capacity measurements remain open under #3, #6, #11 and #20.
