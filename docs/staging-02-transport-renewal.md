# Staging-02 retained-key transport recovery

This #7 prerequisite restores authenticated management access to the existing second disposable node. It does not restore public PDS availability, enroll an account or allocate a slot. [The original staging renewal](live-staging-certificate-renewal.md) remains a separate fixed-node operator.

## Scope and operation

`deploy/homelab/renew-staging-02-node.py --confirm-staging-02` runs on Kvant with its existing private `~/.local/state/prichal-staging-02-bootstrap/` custody. It permits only node `60684f10-c6b2-4833-a0b8-fb9f96a7869b`, loopback management PostgreSQL port 35437, database `pds_local` and role `pds_staging`. URL query overrides are refused except the existing `sslmode=disable` loopback setting. No private key is copied between hosts.

The private `transport-renewal/` directory must already contain a reviewed, mode-0700 `node-admin` binary built with the pinned Go toolchain from `backend/cmd/node-admin`. The coordinator and existing authority/database files stay private. The node leaf may be expired; the CA must have more than two days and worker leaf more than one day remaining. Insufficient authority/workload validity refuses rather than silently rotating or replacing them. Registry custody must hold no placement or one well-formed placement, and must read back unchanged after publication. Authenticated capacity must report exactly those IDs, one slot and three CPUs. The original version required retained deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f`; see [the post-release change](#post-release-custody) below.

The existing `node-admin renew` transaction verifies the retained registry identity and issues a new server leaf for the same node key. The coordinator retains before/candidate/result receipts, verifies the public key is unchanged, sends only public certificates over the fixed pinned SSH connection, checks installed bytes and verifies mTLS capacity/custody. It does not create a bootstrap, edit the node configuration, change a namespace or call Docker. Registry commit precedes guest publication; uncertain completion retains evidence for inspection.

The guest root helper is `deploy/homelab/publish-certificate.py`, installed at `/usr/local/libexec/prichal/publish-certificate.py`. `--staging-02` selects this fixed node and its actual `prichal-agent.service` unit. Without the flag, the original node/unit remain unchanged. The helper checks the retained CA public key, retained node key, exact node subject, server purpose, loopback IP identity and minimum certificate validity before replacing the public certificate pair. It restarts only the selected agent. Activation failure restores the prior pair and attempts to restart the previous agent; an expired prior leaf is not a working transport fallback.

This is an explicit one-shot command, with a nonblocking lock and private receipts, not a timer. It currently renews the node leaf on every confirmed invocation. Inspect current validity and receipts before rerunning. CA/workload renewal, scheduled execution and independent expiry alerts remain separate work.

## Live evidence — October 29, 2026

Read-only inspection found the node leaf expired at 09:55:18 UTC. The agent was active, but both managed native/ingress containers were already stopped with finish timestamps at 02:18:30 UTC. The running-state preflight refused before certificate publication; the cause of the stop was not established. All four managed/private-predecessor container IDs were retained, with restart policy `no`. The native services were not started.

A newly installed root helper and fixed coordinator renewed the retained node leaf successfully. Verified mTLS capacity reported three CPUs, one slot and the same retained namespace. Installed bytes matched the candidate. The new expiry is **October 30, 2026 at 22:18:46 UTC**. CA and worker certificates were unchanged, and no timer was installed.

Before/after native snapshots matched container IDs/states/restart policies/mounts, the native credential digest and node-key digest. Read-only SQLite inspection found zero accounts and zero active writers. Public HTTPS health returned **502**. The earlier management ready checkpoint and October 28 public success are historical; this renewal does not establish current native health, public readiness or federation.

Candidate SHA256 values:

| Component | SHA256 |
| --- | --- |
| node-admin | `a86a4f2ac89e9a74e81cb879a02e6731e91ea792a8fe4bd3f21a361f04a0f581` |
| Coordinator | `b612c6dc75ac0bebe56e080ed8b337596c7c66fc52056be3384390247aa430d1` |
| Guest publisher | `90c38f321787678e73cf12011f1afb6a4a5f4ff937a16c13756ea2db9438b660` |

Private receipts, public certificate copies, binaries and native snapshots remain under Kvant's `~/.local/state/prichal-staging-02-bootstrap/transport-renewal/`. The publisher was absent before this operation; no previous helper was overwritten. Existing native/agent binaries, runtime configuration, Cloudflare/DNS, Caddy routes and forwarding units were unchanged. No provider capacity or spend was added. Earlier management snapshot proofs retain their original dated scope; the new registry certificate is not in those earlier archives.

## Post-release custody

The [fixture retirement](managed-fixture-withdrawal.md) on October 30 released the only placement, so registry custody and agent capacity are both empty. The original coordinator refused that state. Node leaves last 24 hours, so the next renewal also has to work after a fresh allocation places a new deployment on this node. The coordinator now records the registry's placements before renewal (none or one valid UUID), requires the same list after publication and requires authenticated capacity to report exactly that list. A placement created or released during renewal, a second placement, a malformed ID, a foreign capacity entry or a changed slot/CPU measurement refuses after registry commit and guest publication, the same point as before. The receipt reports `instanceIds` instead of the fixed `instanceId`.

Older renewal binaries cannot read the schema-42 registry. The control directory's `node-admin` must be built from a schema-42 merge before use.

## Validation and next step

Eight original/new staging certificate tests passed, including real OpenSSL foreign trust/key/subject refusal, fixed-registry custody and injected activation rollback. The new CI test uses standard-library Python and OpenSSL. The initial default local Python lacked cryptography for the older tests; running those with the system Python passed. Full pinned smoke passed (56 Node checks, frontend build, Go vet and all Go race packages). Local verification PostgreSQL had zero generated smoke/recovery databases and was stopped with its volume retained. Hosted CI remains separate.

Canonical Machinum report and Kvant briefing were committed as `79126d5`; the audit had no integrity errors. Guarded briefing preview prepared six outputs without applying them. No website publication is claimed.

Pause by withholding invocation; there is no timer to disable. Preserve keys and all receipts. After uncertain publication, inspect registry/installed certificate and current mTLS before issuing another same-key leaf. Do not bootstrap a replacement identity or restore an expired leaf as a transport repair. Investigate the stopped native state and qualify any resumption separately before treating the endpoint as ready. Fresh managed allocation/forwarding handoff, verified handle bindings, DNS/provider failure and ongoing renewal/withdrawal remain #7 gates; #8 follows #7.

Subsequent [guarded native resumption](managed-staging-resumption.md) investigated the boot-time stop and restored the same managed containers through restricted-agent reconciliation. Fresh public checks passed; this report's stopped/502 result describes transport-recovery time, not the later observation.
