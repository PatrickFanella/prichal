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

The ready message identifies a healthy local PDS. Public DNS, TLS ingress, account identity and downstream federation remain unverified. The internal network deliberately prevents external identity access and host ingress.

`./scripts/dev.sh smoke` verifies worker reconstruction after host success, one placement and one runtime ID, pending-health behavior, kind-aware replay, stale measurements, namespace accounting, measured bounds and TLS key/identity rejection. Local browser qualification created one healthy PDS and refused another request after capacity filled. Phone, tablet, landscape and desktop widths had no horizontal overflow.
