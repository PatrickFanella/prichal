# Local restricted node agent

The agent is a separate executable with Docker access. The API and deployment test worker have no Docker socket or host credentials. This qualification implementation requires explicit local mode and a loopback listener. It does not enable production provisioning.

```sh
./scripts/agent-local.sh init
./scripts/agent-local.sh run
# In another terminal, choose a fresh lowercase UUID for an owned local fixture:
./scripts/agent-local.sh ensure aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa
```

The init command generates a dedicated, seven-day local CA and agent/worker certificates in ignored `.local/node-agent/certs`. Repeated init preserves existing material and rejects an incomplete certificate set. These are development credentials. Keep the CA private key out of production nodes; production signing and enrollment remain separate work.

The listener requires TLS 1.3 and a verified client certificate from the dedicated CA with worker identity. The lifecycle operation is `PUT /v1/instances/{uuid}` with `{}`. Requests cannot supply images, commands, paths, environment, mounts or ports. Invalid paths and unknown fields fail before Docker execution.

The agent uses the fixed official PDS digest and Docker Engine API 1.44 over an operator-configured Unix socket. It creates an instance-specific internal network, two labeled named volumes, and a container with a stable name. Runtime limits are one CPU, 768 MiB and 128 processes. It drops capabilities and sets no-new-privileges. It refuses existing resources with conflicting ownership and refuses container security/storage settings that differ from policy.

Agent state contains the PDS JWT secret, admin credential and PLC rotation private key in private files. Atomic file publication and directory synchronization preserve the winning credential set after retries. Credential values never appear in the response. The operator's Docker daemon remains privileged and can inspect container configuration; Docker access is an operator trust boundary.

Repeated ensure calls inspect and reuse the existing container. They start it if stopped and preserve storage. There is no delete operation. A timeout can leave resources in place; retry the same UUID to reconcile them. Do not manually remove volumes to recover an uncertain request.

The local PDS uses `.test` handles, blocked PLC access and no crawlers. A running container is not public readiness. The internal network does not provide host ingress despite a loopback port binding in the container declaration; local health qualification uses a fixed `docker exec` probe. Managed local TLS ingress, measured placement and node registration are described in [local agent provisioning](local-agent-provisioning.md) and [local node enrollment](local-node-enrollment.md). Disk quotas, certificate rotation and managed account fixtures remain pending.

Tests use a fake Docker HTTP server to inspect fixed configuration and prove one container after a repeated ensure. Additional tests reject arbitrary image/path requests, unauthenticated HTTP and credential symlinks, and preserve credentials after agent reconstruction. `./scripts/dev.sh smoke` runs these tests with the race detector. The optional golangci-lint executable is unavailable on this workstation; Go vet runs in the required gate.

Docker API reference: <https://docs.docker.com/reference/api/engine/>.
