# Deploying a local PDS through the worker

Start the local database, issuer, API and web interface. Enroll the local node using [node enrollment](local-node-enrollment.md), then start its agent. Configure the separate worker with the dedicated client certificate:

```sh
PDS_WORKER_AGENT_CA_FILE="$PWD/.local/node-agent/certs/ca.crt" \
PDS_WORKER_AGENT_CERT_FILE="$PWD/.local/node-agent/certs/worker.crt" \
PDS_WORKER_AGENT_KEY_FILE="$PWD/.local/node-agent/certs/worker.key" \
./scripts/dev.sh worker
```

In the browser, choose `Create a local PDS`, enter a server name and submit. `Simulate deployment` remains available and never creates a container. Schema version 5 stores the operation kind, so worker configuration cannot silently change a queued simulation into a real deployment. Reusing a request key with another kind is a conflict.

The worker reads the enrolled node endpoint and key pin from PostgreSQL. Its HTTPS client checks the dedicated CA, node certificate identity and public-key hash before calling the agent. Redirects, unregistered keys and nonlocal endpoints are rejected. The public API loads neither these credentials nor the Docker socket.

Before placement, the agent reports Docker CPUs, total memory and all retained managed namespaces. Local capacity is capped at two instances, then reduced to reserve one CPU and 2 GiB of memory for host overhead. Management subtracts retained namespaces outside the node's placement inventory and clamps to the operator ceiling. Measurements expire after one minute; a failed verification invalidates them. Reservations persist through failures. The node also checks its capacity before creating a new container.

These measurements are local admission checks. They do not enforce volume quotas or prove production workload capacity. The two-instance ceiling avoids commercial sizing claims. Unrelated containers remain untouched.

A deployment moves through queued, configuring and checking before internal Docker health becomes ready. Health checks can wait for up to two minutes from submission, with one-second durable snoozes. Other errors have three attempts. Resource creation always uses the original operation UUID; a lost response retries reconciliation rather than creating another namespace. Failure retains storage and reports capacity exhaustion or uncertain completion without raw host errors.

The ready message identifies a healthy local PDS. Public DNS, publicly trusted certificates, account identity and downstream federation remain unverified. The internal network deliberately prevents external identity access; controlled Caddy ingress provides local host access.

`./scripts/dev.sh smoke` verifies worker reconstruction after host success, one placement and one runtime ID, pending-health behavior, kind-aware replay, stale measurements, namespace accounting, measured bounds and TLS key/identity rejection. Local browser qualification created one healthy PDS and refused another request after capacity filled. Phone, tablet, landscape and desktop widths had no horizontal overflow.

## Local managed ingress

Schema version 6 allocates each managed hostname and local endpoint in the intent transaction, independently of placement. The agent runs the pinned Caddy image with a read-only generated config, disabled admin API, bounded resources and a loopback port. It attaches Caddy to the PDS's internal network and a separate ingress network. Only the instance's hostname routes to its PDS; unknown hosts return 421.

The local port is deterministic from the instance ID. A collision with an existing service fails safely and retains the PDS resources. It does not choose another endpoint silently. This local port allocation is not a production DNS or load-balancer design.

Agent readiness now requires both the PDS's internal health and a successful health request through Caddy. The browser exposes the local endpoint for checking/ready operations. Existing internally-ready operations keep their earlier evidence message; migration preserves their allocated identity and data.

```sh
node scripts/check-ingress.mjs \
  https://bca4cb84-cfb2-4e58-afc6-26c5edad9ade.localhost:44292 \
  .local/node-agent/bca4cb84-cfb2-4e58-afc6-26c5edad9ade-public.crt
```

The qualification checks TLS using the fixture certificate, the advertised OAuth issuer and PDS service DID, pinned PDS health version, unregistered-host rejection and WebSocket upgrade. Public DNS, publicly trusted certificates, federation and live TLS validation remain deferred. The node capacity budget includes each ingress container's 128 MiB and quarter CPU; ingress containers do not count as additional PDS namespaces.

## Local TLS identity correction

Schema version 7 changes local agent endpoint schemes from HTTP to HTTPS while preserving hostnames and reserved ports. It records the correction in operation history. The pinned PDS derives its advertised URL from hostname configuration; its hostname now includes the reserved local port and its service DID encodes that port. TLS ingress serves the same URL. Public identity access remains disabled.

Each instance has a separate seven-day self-signed TLS identity in a private, atomically published agent file. The agent refuses mismatched, expired, publicly readable or symlinked identity files. Caddy receives only the fixed certificate/key file and configuration through read-only mounts. Reconciliation validates exact mount source/destination pairs without depending on Docker's inspection order, and verifies the PDS's fixed environment.

Export only the public certificate for local checks:

```sh
./scripts/agent-local.sh certificate bca4cb84-cfb2-4e58-afc6-26c5edad9ade
```

The check script trusts that certificate only for the requested fixture. No system or browser trust root is installed. Browser certificate trust and production certificate renewal remain separate work. Expired local identities fail closed instead of silently generating a new key.

The correction helper accepts only an empty, identity-disabled local fixture on a Unix Docker socket. It saves private container inspection snapshots, stops ingress and its writer, retains the old containers under `-before-local-tls` names and retains the old Caddy configuration. No volume or credential is removed. The agent can reuse the same namespace only when all prior writers are stopped; running, paused and restarting writers block replacement. Populated identities cannot use this helper.

For rollback, stop the new ingress and PDS first. Preserve them under other names, restore the old container names and saved Caddy configuration, then start only the old writer and ingress. Restore management endpoint metadata deliberately for the old HTTP transport. Never run both writers against the same SQLite volumes. The snapshots include private environment values and must remain in ignored owner-only storage.
