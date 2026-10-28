# Retained managed routing inventory

This is the second implementation unit of #7. It reserves first-level names under `subcult.tv` and prepares private routing candidates. It does not publish DNS, install forwards, reload Caddy, change a PDS endpoint or establish public readiness. #7 and #126 remain open.

## Durable identity and readback

Schema 39 adds `managed_public_endpoints` and `managed_edge_observations`. A reservation retains the deployment UUID, exact generated `pds-<UUID without hyphens>.subcult.tv` name, unique forward port in 35000–35999, operator identity and timestamp. It is independent of placement and is retained on retry. At most 100 endpoints may be reserved through this store. Existing private deployments receive no implicit reservation or runtime rename.

Normal reservation requires queued, unplaced native intent. A separate `reserve-empty` operation accepts only the designated loopback-35437 staging-02 registry, an explicit expected node and `--confirm-empty-fixture`. It requires a ready private native fixture, matching authenticated healthy status and fresh zero-account usage. This reserves metadata only; runtime conversion needs its own fenced procedure. Never use the fixture for real accounts.

The node agent's authenticated `GET /v1/instances/<UUID>/certificate` route inspects retained healthy ingress resources and returns only the existing public certificate, digest, instance and endpoint. It never creates or reconciles resources. The client uses the current registry-pinned node key, strict bounded JSON and the existing no-redirect mTLS transport. The certificate must contain exactly the valid public leaf for `<deployment UUID>.localhost`; private PEM material is rejected. The legacy public-pilot handler does not gain this route.

Observation locks the retained deployment, reservation, placement and current enrolled node, then appends a public-certificate receipt with a stable request UUID, node-key hash and actor. Repeating that key with the same actor/node returns the original receipt without a new transport operation; a new observation uses a new key. Old receipts are retained as evidence, not selected as a fallback when the latest receipt is invalid.

Export reads the whole reserved inventory and refuses a partial result. Every endpoint needs its latest valid certificate, unchanged current placement and enrolled node key, eligible deployment/node states and a readback no older than 15 minutes. Its positive generation is the global observation sequence from the same SQL snapshot. `expiresAt` is the earliest readback deadline or certificate expiry; the renderer rechecks this deadline. An expired bundle needs a fresh observation and export. This expiry is a candidate-generation gate; no live publisher or automatic route expiry exists yet.

Recovery holds are checked before and after waiting or external work and before commit. An allocator-named `pds_recovery_<32 hex>` copy also requires a reconciled recovery receipt; a raw restored copy is insufficient. Export checks schema readiness without migrating and runs read-only. Restore qualification compares the complete management snapshot before testing both raw-copy and held-copy refusal.

## Operator commands

Build with the pinned toolchain into ignored private storage:

```sh
cd backend
go build -o ../.local/managed-routing/managed-route-admin ./cmd/managed-route-admin
go build -o ../.local/managed-routing/managed-edge ./cmd/managed-edge
```

Load private configuration through the established custody path. `PDS_MODE=local`, or production mode with `PDS_STAGING_PROVISIONING=true`, is required. `PDS_DATABASE_URL` must identify the designated loopback management registry; normal writes require `pds_local`. Observation and empty-fixture reservation require the complete worker-agent CA/certificate/key tuple. Invalid flags, UUIDs, mode, registry scope and credentials are refused before database work.

For queued intent, then an activated native placement:

```sh
.local/managed-routing/managed-route-admin reserve --deployment "$DEPLOYMENT_ID"
.local/managed-routing/managed-route-admin observe --deployment "$DEPLOYMENT_ID" --node "$NODE_ID" --key "$REQUEST_ID"
.local/managed-routing/managed-route-admin export --out .local/managed-routing/new-bundle
.local/managed-routing/managed-edge --inventory .local/managed-routing/new-bundle/inventory.json --certificates "$PWD/.local/managed-routing/new-bundle/certificates" --out .local/managed-routing/new-candidate.Caddyfile
```

The current API does not call this reservation store; automatic allocation is the next unit. The explicit staging-02 metadata procedure replaces the first command with:

```sh
.local/managed-routing/managed-route-admin reserve-empty --deployment "$DEPLOYMENT_ID" --node "$NODE_ID" --confirm-empty-fixture
```

Bundle creation requires a new directory. Directories are mode 0700 and files mode 0600. Certificates are validated before output, copied under canonical UUID filenames and synced; `inventory.json` is written last. Partial output is retained for inspection and never overwritten. Handle exports are empty until authoritative account/identity bindings are available. Keep the bundle private and deliver it explicitly to the future restricted publisher; customer input is not an authority source.

## Native evidence and limits

On October 28, 2026, the candidate agent was installed only in the second disposable Kvant guest. Its preceding binary remains `/usr/local/libexec/prichal/node-agent.before-routing`. Restarting `prichal-agent.service` preserved native container `9af8dcea60d6`, ingress container `38cefcb10934`, the placement and both native volumes. Only the separate staging-02 management registry migrated to schema 39.

Fresh authenticated status/usage allowed an explicit empty-fixture reservation for deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f` on node `60684f10-c6b2-4833-a0b8-fb9f96a7869b`: `pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv`, forward port 35000. Public-certificate observation and identical-key replay passed. A fresh second observation produced generation 2 with expiry 15 minutes after readback; the private renderer accepted that current bundle. These candidates expire and must not be treated as current deployment authority later. The native endpoint remains `https://2395105c-9958-47b5-ae8b-5b0424cbfc4f.localhost:45109`; no DNS, forward, public route or account was created.

The full PostgreSQL snapshot was restored into an allocator-named disposable database. All 65 public tables' row counts/logical hashes and all sequences matched, including reservations and both immutable observations. Raw/unmarked and explicitly held restored copies refused export without producing a bundle or changing routing rows. The source stayed unchanged; the exact disposable copy and plaintext dump were removed. The final snapshot SHA256 was `b824d72b1dcf88f3597ea1f9ec9dd0aa61b55df08a3a4687a7f4431b4ae00d89`. This is same-host management snapshot qualification, not off-host recovery or a native SQLite/blob restore.

Private scripts, receipts, candidate binaries and bundles are under `.local/managed-routing/` here and `~/.local/state/prichal-staging-02-bootstrap/managed-routing/` on Kvant. These are worktree candidates, not published release artifacts. Canonical infrastructure evidence belongs in Machinum. Local tests cover reservation/replay races, stale/latest-invalid certificates, changed node identity, incomplete inventory, late recovery holds, output conflicts and traversal, strict CLI scope, public-only certificate responses and migration without backfill. The repository smoke gate and hosted CI are separate results.

## Rollback and remaining gates

Agent rollback can atomically restore its retained preceding binary and restart only the guest agent after checking the expected paths and retained runtime identities. Do not bootstrap over the guest, recreate the PDS, remove volumes or restore another writer from its disk.

Schema 39 is additive, but older management binaries reject a newer schema. Do not drop routing evidence or force the schema version backward to run them. Database rollback needs a reviewed compatible snapshot/restore procedure and fencing of all writers. The qualification script drops only its exact allocator-owned recovery copy; it never drops the source.

Next work in #7 must join reservation to durable API intent, carry the retained public identity into native runtime policy, implement the restricted publisher with conflict/rollback checks, and persist public DNS/TLS/issuer/WebSocket readiness. It must qualify failure and same-identity recovery on designated staging capacity. Native enrollment/default handles remain #8 work. API and ordinary workers receive no host, Docker or DNS administration.
