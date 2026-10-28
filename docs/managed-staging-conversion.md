# Empty staging-02 managed conversion

Scope: deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f`, node `60684f10-c6b2-4833-a0b8-fb9f96a7869b`, only the owner-authorized second disposable Kvant VM and loopback-35437 registry. Preserve the occupied first node, public pilot, unrelated services and the original second-node fixture's keys, volumes, containers and private configuration. Do not enable accounts or install public DNS/routes.

PR #206 merged as `ac80348adb8a89f5baa7cbcca82506dd3fee6760` after its required check passed. The fresh native path refuses this retained private fixture. Conversion therefore needs an operator procedure; it cannot be an implicit reconciliation fallback.

## Procedure and authority boundaries

1. Add Linux process ownership fencing shared by the agent and the local converter. Keep public native network/configuration names separate from retained private predecessors. Permit read-only production usage only when bound to the exact generated native identity; preserve the local guard. Verify lock contention/symlink refusal, private/pilot compatibility and actual pinned-image usage.
2. Add schema 40's immutable conversion record and an operator-only confirmation command scoped to the second registry/deployment/node. Bind the original private identity, node key, verified cold native/management backup digests and host receipt to authenticated managed status, zero-account usage and retained ingress certificate. Commit the conversion checkpoint and same-deployment managed job atomically, with exact-key replay and recovery fencing. Public readiness stays pending.
3. Build candidates and inspect the live node/registry/consumers. Upgrade only the dedicated guest agent while still private, retain its predecessor binary and verify unchanged native IDs. Confirm the current certificate validity, process lock, private status, zero accounts and retained public reservation. No worker runs during native conversion.
4. Stop the guest agent and obtain its exclusive ownership lock. Stop ingress before the native writer; verify exact IDs and stopped state. Create exclusive private cold archives of native data/blobs/state and a full management snapshot. Restore the native archive into an independent private directory, compare file hashes and verify SQLite integrity/zero accounts before changing identity.
5. Rename the stopped original containers into fixed predecessor names; retain the private network/configuration and both data volumes. Install the generated identity receipt exclusively and restart the same agent with the managed runtime opt-in. Reconcile the same UUID through authenticated managed transport, retaining credential/TLS hashes and exactly one active writer. Refuse missing backup evidence or changed resource identity.
6. Confirm management state through the scoped operator command, replay its key without another job, and run the real managed River worker with injected uncertain completion/reconstruction. Verify one placement/namespace, checking state, empty serving URL, public descriptor/OAuth identity, private WebSocket behavior, closed registration, unknown-host refusal and fresh public-certificate observation. No public-readiness claim follows from these private probes.
7. Record rollback paths and remaining public gates in this report/progress and canonical Machinum evidence. Run the full pinned smoke gate, migration/snapshot scope checks and documentation/diff checks. Keep all retained predecessors and archives; stop local disposable verification infrastructure after checking for generated databases. Hosted CI remains separate.

## Rollback contract

Fence the worker and agent before any rollback. Retain any new managed native data separately, stop the new native/ingress containers and verify their exact identities. The original containers, private configuration and network must remain available. Returning to private mode requires restoring the verified cold data/state and exact management checkpoint without a second writer, then checking the original private identity. Do not regenerate credentials, overwrite a populated PDS or force a schema downgrade. Qualification of the actual rollback is a separate recorded result; archive existence alone does not establish recovery.

## Qualified result — October 28, 2026

The designated empty fixture now advertises `pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv`. Management is **checking**, with an empty serving URL, one placement and one conversion record. No public route, DNS record, forward, account or default handle was installed. #7 and #126 remain open. The original staging node and separate public pilot were outside the conversion scope.

The guest agent holds a process-wide Linux file lock. The root converter requires the agent to be inactive, takes the same exclusive lock, and checks the fixed guest, deployment, node, original container IDs, pinned images, volume names and disabled automatic restart. Its `prepare`, `activate` and `abort-before-create` actions require the retained request key, management backup digest and expected agent binary digest. There is no HTTP conversion route. Managed networking and ingress configuration use separate names so the stopped private predecessors keep their original network and configuration.

Before changing identity, the procedure stopped ingress and then the native writer. It independently restored a private cold archive of native state, SQLite data and blobs, matched all regular-file hashes, and passed SQLite integrity/zero-account checks. The archive includes native credentials and TLS material and must remain private; the enrolled node key stays in its separate custody path. A full management snapshot was also retained and restored before conversion.

The pre-creation abort was exercised: after renaming predecessors and installing managed identity/configuration, the procedure removed only its own receipt/override and restored the original names. Authenticated private readback returned the private endpoint with stopped native resources. The original containers were deliberately not restarted, preserving the cold data. The same operation then reactivated managed identity and the restricted agent created the new native resources. This qualifies the pre-creation abort; it does not qualify recovery after new native writes.

| Resource | Retained result |
| --- | --- |
| Operation | `bb06b240-d4b5-4d42-91be-aa9c198ebf4e` |
| Original native / ingress | `9af8dcea60d6` / `38cefcb10934`, stopped under fixed predecessor names |
| Active native / ingress | `a7f519a5cc68` / `e86b64f782a4` |
| Storage and native credentials | Same data/blob volumes and unchanged credential-file digest; one active native writer |
| Ingress certificate SHA256 | `851997f4b23c86fe6bd981e525c5f2bb22ec7bb2e9797a26518284c732624cac`, unchanged |
| Native cold archive SHA256 | `c2a4f2377dc18a499695087bb75c756c9be4affb536a91cc3269f9a1e0867aed` |
| Management before SHA256 | `584ea26937023732394f5905c11b3f6cdb7cb6b1bc079dd45e9a76ae126cd59e` |
| Management after SHA256 | `28a544a3fa8519391aaca23afbb1dee4d698ce7e816433e9d34cd3a6b3472368` |

Both management archives restored all 66 schema-40 public tables and every sequence into separate allocator-owned databases. The before copy contained zero conversion rows; the after copy contained one. Raw/unmarked and recovery-held copies refused routing export, the source stayed unchanged, and both exact disposable copies were dropped. The archives remain private. This is same-host qualification: guest storage and the Kvant copy share the host's physical failure domain.

## Management confirmation and replay

Schema 40 adds `managed_native_conversions` without backfilling identity or granting conversion authority. The append-only application path records the original private endpoint, placement/node-key binding, operator and custody digests. `managed-conversion-confirm --proof FILE --confirm-staging-02` requires explicit production staging mode, the fixed deployment/node and loopback `127.0.0.1:35437` registry with database `pds_local` and user `pds_staging`. It reads a private bounded strict JSON proof, hashes its exact bytes, and requires authenticated managed health, fresh zero-account usage and the unchanged public ingress certificate. It commits the conversion, configuring checkpoint, progress event and managed River job in one transaction. Changed custody, unfinished work, changed node keys or recovery holds refuse confirmation.

The live command and exact-key replay returned the same receipt with one managed job. The actual River worker then lost its checkpoint after a successful native reconciliation response, was reconstructed and reconciled the same resources. It retained one placement/namespace and reached checking with an empty serving URL. The agent had already created managed resources during the operator procedure; this was worker reconciliation/replay, not fresh API-to-native allocation.

An authenticated public-endpoint observation produced sequence 4. Export succeeded, but its 15-minute bundle has expired and must be refreshed before publication. Converted fixtures now require public-endpoint certificate readback; a retained private certificate receipt alone cannot authorize the converted path.

Private TLS probes against the actual managed ingress passed PDS `0.4.5036` health, exact service DID and OAuth issuer/authorization/token endpoints, zero repositories, native/OAuth signup refusal, unknown-host rejection and WebSocket upgrade. These used the existing private ingress certificate and transport. They do not qualify public DNS, TLS, external routing, outbound PLC/crawler reachability or real account login.

## Verification, custody and remaining gates

The pinned smoke gate passed with `GOFLAGS=-p=2`: 56 Node checks, frontend build, Go vet and all Go race packages. Focused tests cover conversion/replay races, rollback on job failure or late recovery hold, changed custody/accounts/node identity, no migration backfill, process ownership/symlink refusal, production usage identity binding and fixed CLI scope. The optional real pinned-image usage check passed bound managed identity and refused missing/wrong identity. Six converter boundary tests and three snapshot-scope tests passed. Hosted CI is separate.

Initial operator preflights refused a historical systemd PID and a lowercase Docker missing-object response; those checks were corrected before native conversion. A private controller quoting error failed before remote execution. No guard was bypassed. Review subsequently required restart policy `no` for predecessors and exclusive service-override creation; readback confirmed the actual retained containers meet that policy.

Private scripts, source hashes, binaries, archives, proofs and receipts are under `.local/managed-conversion/` here, `~/.local/state/prichal-staging-02-bootstrap/managed-conversion/` on Kvant, and `/var/lib/prichal-agent/managed-conversions/<operation UUID>/` in the guest. Installed agent SHA256 is `63082ef15627e693d5d8836634de7cfcbb3d41a1307b935f88c73f340d297fa0`; its preceding binary remains `node-agent.before-managed-conversion`. These are worktree candidates, not published releases. Canonical infrastructure evidence is maintained in Machinum.

Do not restore an older private agent over managed state. Fence the worker and agent before reviewed cold recovery; preserve new writes and both sets of resources, restore a compatible native/management checkpoint, and verify one writer and the expected identity. The automated abort refuses after new managed resources exist. Full post-creation rollback, independent/off-host recovery and unattended certificate renewal remain unqualified. The node transport leaf expires October 29, 2026 at 09:55:18 UTC.

Next #7 work is the restricted publisher and durable public DNS/TLS/issuer/WebSocket readiness, including controlled failure and same-identity retry. Keep account enrollment closed until #8.

Subsequently, PR #207 merged this conversion as `939dfb2624ca3ad9e607aeb4b054e2bd9f588693`. The [initial managed staging publisher](managed-staging-publication.md) now records a public route and its rollback/republication qualification. The conversion-time no-public-route observations above are historical; management still remains checking with no serving URL.
