# Managed retirement record

Schema 42 adds an explicit management boundary for the owner-authorized empty staging-02 retirement. It retains deployment identity, endpoint/forward reservation, observations and public-readiness history. The migration creates no retirement records and changes no existing deployment or placement. This unit implements the management record and fixed-scope operator command; it has not migrated or retired the live fixture.

`prepare` requires a ready generated native deployment, its current pinned enrolled node, no unfinished deployment work or enrollment operations, authenticated healthy managed status and fresh zero-account usage. An operator supplies verified native and management backup digests through private custody. These digests record the operator's assertion; the command cannot verify archives on another host. One transaction records the request/node key/actor/backups, clears the serving URL, marks the deployment failed with explicit retirement guidance and appends one event. It keeps the placement reserved and blocks routing export. Managed worker reconciliation checks retirement authority before and after native calls. Preparation does not stop a process, withdraw a route or free a slot; operators must fence other work and install the separate native reconciliation hold before removing resources.

`release` requires the same request/actor, retained failed state and empty URL, unchanged placement/node key and no unfinished work. It records independently reviewed edge-withdrawal and native-retirement receipt digests. Authenticated node capacity must report the designated one-slot/three-CPU host with no retained native namespaces. Only then does one transaction mark the record released, remove its placement and append the release event/guidance. This measurement does not prove route withdrawal or a durable native hold; those are required operator receipt checks, not inferred from an empty container list.

Exact request replay returns the retained record without repeating native work or adding events. Changed custody/actor/request or release digests conflict. Recovery holds are rechecked around external calls and before commit. A late hold or changed node key keeps the placement and preparation intact. Released endpoints are excluded from active complete-inventory export, while their names/forward ports remain reserved. Any new unqualified endpoint still refuses the entire export. A fresh qualified endpoint can export alongside retained retirement history. This is a bounded staging retirement path, not general customer deletion or account-data destruction.

## Fixed operator command

Build `backend/cmd/managed-fixture-retire` with the pinned toolchain into ignored private storage. Execution requires explicit production staging, the existing private worker-agent certificate tuple and the fixed `127.0.0.1:35437/pds_local` registry as `pds_staging`. Schema 42 must already be present; the command checks readiness without applying migrations. Deployment/node IDs are fixed to the authorized original staging-02 fixture and cannot be overridden.

After verifying the [preserved backups](managed-fixture-retirement.md) and current transport validity:

```sh
managed-fixture-retire prepare --key "$RETIREMENT_KEY" --native-backup-sha256 "$NATIVE_BACKUP_SHA256" --management-backup-sha256 "$MANAGEMENT_BACKUP_SHA256" --confirm-empty-staging-02
```

Then fence native reconciliation, withdraw the exact public route through a reviewed restricted publisher path, remove only the fixed retired container resources and verify the actual route/namespace absence. Preserve original volumes, keys, archives and receipts. Only after those steps:

```sh
managed-fixture-retire release --key "$RETIREMENT_KEY" --withdrawal-sha256 "$WITHDRAWAL_RECEIPT_SHA256" --native-retirement-sha256 "$NATIVE_RETIREMENT_RECEIPT_SHA256" --confirm-empty-staging-02
```

The API and ordinary worker gain no host, Docker, SSH or DNS credentials. The command performs no host mutation and offers no customer HTTP route. It does not implement the publisher withdrawal/native-removal steps. Do not fabricate their digests or use this command to bypass them.

## Compatibility and remaining work

Schema 42 is additive; older binaries reject it. Preserve a verified management snapshot before a live upgrade. Do not drop retirement records or restore a pre-retirement source over later writes to roll back. Preparation deliberately fails serving and is not automatically undone; inspect uncertain completion and replay the same request or continue the authorized retirement with compatible binaries. A released record remains historical when later node state changes and is not fresh capacity proof for another allocation.

Local PostgreSQL/race checks cover preparation/replay, no premature release, preserved identity/history/forward reservation, fresh complete export after retirement, populated/foreign/bad-custody refusal, changed node key and recovery holds introduced during native inspection. CLI checks refuse ambiguous phases, target overrides and foreign registries before database work. Existing migration fixtures reconstruct only empty retirement metadata inside allocator-owned disposable databases. Live migration, exact route withdrawal, native hold/removal, slot release and fresh API-to-native allocation remain unperformed in this unit.
