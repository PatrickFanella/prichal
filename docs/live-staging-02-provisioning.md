# Second staging node and restricted provisioning — October 28, 2026

The owner authorized a second disposable Kvant node: three vCPUs, 4 GiB RAM, a 40 GiB disk and $0 provider spend. The occupied first staging node and the separate public pilot were preserved. This report qualifies private provisioning for #6; it does not qualify managed public DNS/TLS, account enrollment, federation or production release artifacts.

## Allocation and custody

`prichal-staging-02` has its own independent qcow2 disk and keys under `/mnt/fast1/apps/prichal-staging-02`, mode 0700. Admission checked the required SSD UUID, more than 50 GiB actual free space, more than 8 GiB available memory and unused loopback forwards. The existing pinned Ubuntu image SHA-256 matched before conversion. Startup verifies the retained bootstrap identity and independent 40 GiB disk. Its user unit caps CPU at 300% and host memory at 5 GiB. Existing Kuznya and Prichal VM paths and services were not changed.

SSH is only `127.0.0.1:22270`; a separately supervised pinned-SSH tunnel exposes the mutual-TLS agent at `127.0.0.1:22271`. Ansible core 2.19.5 configured Ubuntu 24.04, Docker, the non-login agent user and the private firewall. Repeated convergence returned `changed=0`, `failed=0`, `unreachable=0`. The agent runs with explicit production-mode staging opt-in and a one-slot ceiling. Its identity key was generated inside the guest and stayed there; only the public CSR left it.

The independent registry is `prichal-staging-02-registry-postgres-1`, PostgreSQL 17 pinned by digest, separate volume, loopback `35437`, 512 MiB and one CPU. Its database remains `pds_local` for bounded qualification tooling. It is neither the first staging registry nor the hosted management database. Dedicated PKI, database credentials, source/binary hashes, scope and qualification receipts are private under `~/.local/state/prichal-staging-02-bootstrap/` on Kvant. Bootstrap cannot replace an existing scope. Its node leaf is short-lived; this disposable fixture has no unattended renewal or backup claim.

Node `60684f10-c6b2-4833-a0b8-fb9f96a7869b` enrolled with a fresh one-time bootstrap. Reusing the consumed bootstrap was rejected. Its initial certificate SHA-256 is `a2a5b1c011ec2c18200016789d1927a2dc4f4eb483b12b2c94e7ff3082c01ff1`.

## Native management/worker drill

The opt-in qualifier invokes the actual production-mode owner deployment handler, durable PostgreSQL/River queue and staging worker against the real guest agent. It discovers the real HTTPS Gitea issuer, but uses a synthetic client configuration and an explicitly seeded synthetic management session. No real Gitea account is impersonated, no authorization-code/token exchange is claimed and no browser login is qualified. The synthetic session is revoked after the drill. Existing #2 authentication acceptance and later #8/#24 journeys remain separate.

The owner handler committed deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f` to the separate staging queue. After the real agent created native resources, the qualifier withheld the successful agent reply from the worker checkpoint and canceled the worker context. This is an injected uncertain-completion interruption followed by worker reconstruction, not an OS `SIGKILL` test. PostgreSQL remained at `configuring`. A new real River worker consumed the retained job, reconciled the same instance and reached `ready`; original-key HTTP replay returned the same deployment. Exactly one placement and one native namespace remained.

Post-drill guest inspection showed native container `9af8dcea60d6`, ingress `38cefcb10934`, and exactly the deployment's data/blob volumes. A final agent-binary update/restart retained both containers. The endpoint is private `*.localhost` identity with a reserved port; public hosting remains unverified.

Live boundary checks rejected certificate-less requests, wrong node identity, wrong key pin, an arbitrary image, an execution path and an invalid instance path. A second UUID was refused at the occupied one-slot ceiling, and an already-canceled reconciliation left the original namespace intact. The first boundary invocation expected 400 for an invalid path, while the existing outer allowlist correctly returned 404; the assertion was corrected and the complete boundary drill passed.

## Source and operational boundaries

The [private control-path contract](private-staging-provisioning.md) describes the explicit opt-in, production authentication rules, agent-only admission, separate worker queue and excluded lifecycle adapters. Its ordinary PostgreSQL/River tests also prove that a default-queue simulation remains queued while a staging job completes. Existing worker recovery fences remain in use. API source loads no worker certificate or Docker socket; native Docker execution remains in the restricted guest agent. The qualification harness is operator tooling, distinct from a confined API deployment.

The runtime candidate was built from this worktree; it is not a published release. Private receipts retain source/binary hashes. PR-head and merged-main CI, release publication, public ingress and the complete managed owner journey are separate gates. No issue closure is claimed before source integration.

For a pause, stop `prichal-staging-02-agent-tunnel.service`, then `prichal-staging-02-vm.service`, and stop only the second registry container if needed. Retain its disk, registry volume, enrolled keys, signer, worker identity and scope together. Restart with the same paths after checking certificate validity; do not bootstrap again or create a second writer from a copied disk. No recovery checkpoint or restore qualification exists for this fixture, so keep it free of real accounts.
