# Populated native member recovery — local qualification

This opt-in qualification restores a fresh disposable PDS containing an owner and one member. It extends the real initial member-window journey with a cold checkpoint, SQLite-aware staging, authenticated encryption and a fresh-storage restore. Ordinary smoke and CI skip the native test unless explicitly enabled.

With the repository's pinned toolchain on PATH:

```sh
COMPOSE_PROJECT_NAME=prichal-storage-qualification ./scripts/dev.sh db
./scripts/dev.sh qualify-member-native-recovery
```

The separate local PostgreSQL, Unix Docker socket and canonical pinned PLC/mail fixture are required. The shared protocol fixture must be stopped initially; this profile refuses to interrupt an existing running fixture. The native broker admits exactly one new source tenant under its measured capacity and existing disposable ceiling. Stopped namespaces still count. The test never prunes retained tenants or volumes.

## Checkpoint and isolation

The underlying native journey creates both accounts through guarded owner/member invitations and retains the exact member migration window. Original synthetic account passwords are passed to the recovery subprocess on stdin, then to fixed in-container native calls on stdin. They are not command arguments or chat output. Both accounts add a signed post with referenced image media before capture.

The driver validates the pinned source image, tenant/project labels, fixed volumes and matching protected credentials. It stops only its fresh source PDS/guard/edge and the shared protocol fixture that it started for this run. It checks that no container still mounts those source volumes before checkpoint operations. Read-only source mounts feed fresh staging volumes. SQLite's backup API normalizes only the staging copy, and integrity checks validate its databases. The checkpoint includes PDS SQLite data, blobs, protected runtime configuration, original TLS/owner lease/member journals and the shared synthetic PLC/mail database.

The normalized archive is authenticated with the existing AES-GCM recovery helper before extraction into fresh volumes. Full file/database inventories match before any restored application starts. The three member host files restore into a separate private agent-state candidate; this profile does not activate that candidate or restore an edge/guard route. The restored PDS and cloned protocol fixture use a new internal Docker network with no published ports. The old source remains fenced while the restored writer runs.

Both accounts must authenticate with their original passwords and DIDs, then create new signed records. The pinned upstream repository verifier checks signatures against the retained local PLC identities; referenced blob bytes, CIDs and hashes match. The restored copy is stopped before source-tree and protected-metadata hashes are compared again. Volumes, encrypted checkpoint, same-machine key and private intermediate files remain retained under `.local/` for review. Labeled-container cleanup is registered before allocation and also runs from the Go harness if its subprocess fails or times out.

## Observed result — October 20, 2026

The final run passed with Go's race detector against fresh tenant `2302beab-0a5a-47c2-b5f6-1b15be9085c9`. Its private receipt is `.local/member-removal-native/11b0a753-571f-4dfb-b81f-5cfc102e052c/cold-recovery/report.json`.

- Six SQLite databases passed backup/integrity and fresh-storage inventory comparison.
- Two repositories, five source signed records and two account-scoped referenced blobs were retained.
- Both original account logins succeeded, followed by two new restored signed writes. Referenced media stayed unchanged.
- The authenticated checkpoint was **595,100 bytes**, SHA-256 `ab9800dbcbff7d72336f64fb8f5b547f72046bf80063a20103fa88e4014b890a`.
- The local recovery interval was **17.518 seconds**, measured from the source stop attempt through verified restored writes. The cold capture timestamp was `2026-10-20T08:36:41.825Z`. Three startup inventory reads retried before readiness.
- Source file hashes and protected metadata matched after the drill. All three new source containers and both restored containers stopped; temporary helper containers were removed by their own `--rm` lifecycle. All source/staging/restored volumes remain retained.
- Independent readback confirmed the same shared protocol container returned to stopped with its original four network attachments. The parent receipt records removal of only the fresh source tenant's temporary attachment.

`cold-recovery/source-evidence.json` records parent commit `07915bcc93901f05c84464a9bcf18b6bc0a020f4`, tested harness/driver/wrapper hashes and independent cleanup readback. An earlier successful rehearsal is retained separately under `.local/member-removal-native/00d9150a-9f50-4eeb-ae6f-08b8b800d5ac/`; its initial startup probe retried successfully. No production data or existing tenant was restored, stopped or changed.

## Failed candidate rehearsal — awaiting native qualification

The separate opt-in command `./scripts/dev.sh qualify-member-native-rollback` prepares the same fresh populated checkpoint and restored instance, with a fixed Docker health check that exits 73. It requires an observed unhealthy candidate and stops both restored writers before resuming the retained source PDS and local identity fixture. Before that resume, source files and protected metadata must still match the cold checkpoint. Both original accounts must then authenticate and produce new signed writes, with their referenced media unchanged. The driver stops the resumed source again; the parent harness retains its independent cleanup fallback.

Only the restored candidate has the injected health check. Its synthetic qualification writes stay on its separate volumes and are not copied back. This is a same-image recovery rehearsal before public cutover; it cannot establish compatibility across release or schema changes. The edge and enrollment guard stay stopped during source authentication and writes. No public route or restored agent is activated.

The first native attempt on October 20 stopped at admission: eight retained tenant namespaces fill the host agent's existing eight-slot ceiling. The test failed before allocating a new source tenant or invoking the recovery driver. The shared protocol fixture returned to stopped with the same container identity and four original network attachments. The private failure log is `.local/issue-audit/member-native-rollback.log`. Existing namespaces, volumes and limits were preserved. **The failed-candidate native path remains unqualified** until an operator provides a permitted free slot or another prepared disposable host; ordinary smoke skips this opt-in path.

## Remaining gates

This is a quiesced same-host fixture. It establishes checkpoint data retention and a measured private native recovery interval, not a scheduled 15-minute data-loss target or public 60-minute recovery commitment. Independent/off-host key custody and storage, host-loss recovery, public DNS/TLS/endpoint cutover, restored-agent activation, downstream/WebSocket transport and production recovery remain unqualified. The checkpoint contains the shared local identity fixture, not the public PLC directory.

The physical 30-day member deadline is not backdated or exercised. Verified organization domains, complete organization termination policy and plan limits remain separate. Management PostgreSQL recovery has its [own snapshot qualification](local-member-deactivation.md#disposable-management-snapshot-recovery--october-20); that evidence does not combine automatically with this native restore. Issues #12, #13 and #117 remain open.
