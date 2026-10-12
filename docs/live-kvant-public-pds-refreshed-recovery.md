# Operated public Kvant pilot checkpoint and isolated restore — October 12, 2026

The current closed pilot checkpoint was retrieved with its independent Dozor recovery key, authenticated, and booted on a separate restricted guest disk. Private protocol and identity checks passed. The restore fixture is stopped and unenabled; no public cutover or account migration occurred.

## Consistent capture and independent custody

Captured source is merged `e6a06f7404d2af3e3a70e9f3edcac882c490ce62`, including recovery, certificate and telemetry operators. Admission measured about 30 GiB available RAM and 411 GiB free on the required SSD. Only the public pilot was paused. Its three timers were stopped, operator jobs were checked inactive, and only its local tunnels/VM were stopped. Jobs were not interrupted. The archive covers its independent disk/SSH state, private transport signer/worker custody, publisher identity/state, current nine user units/resource drop-in, and selected public Almaz/Dozor helper/route/trust/rule snapshots.

The only source symlink was pinned Node 24.15.0; capture materialized that binary. The resulting 124 members were all regular files/directories. Guest capture plus the VM/tunnel restart command measured 64.53 seconds. This excludes later warmup and is not a measured total public outage. All three previously active timers were restored, and full public checks passed afterward.

AES-256-GCM ciphertext is retained on Kvant under `~/.local/state/prichal-public-pds-recovery/20261012-operated/` and Dozor at `/srv/recovery/kvant/prichal-public-pds/kvant-public-pds-20261012-operated.pvm`. Cipher SHA-256 is `27d84355def24423e03044d1b7642b818d5701665f649237de31b95c58fdb19f`; trusted plaintext SHA-256 is `60633f3555dd91dda61b1df73171f71c3e9ed571cb0caeef3badea558353eb3c`. The separate private recovery key remains on both hosts under `~/.local/state/prichal-public-pds-recovery/keys/kvant-public-pds-9f148c55.key`. The preceding checkpoint and parent ownership were preserved.

Dozor's actual ciphertext and key were retrieved and authenticated before restore. New capture/restore receipts are retained on both hosts. Plaintext tar archives and retrieved scratch keys were removed after extraction/qualification; the stopped fixture remains private for inspection.

## Fixed isolated fixture

`prepare-public-pds-restore.py` refuses existing state and validates the fixed archive hash, path prefixes, regular types, duplicate paths and authenticated plaintext before extraction. It retains captured host units as inert files. `restore-public-pds-vm.py` verifies the required SSD, private ownership and independent 40 GiB disk, then boots only `/mnt/fast1/apps/prichal-public-pds-restore-20261012/` using QEMU `restrict=yes`. Its only QEMU forward is loopback SSH 22263. The unenabled qualification unit caps three vCPUs/4 GiB guest memory at 300% CPU/5 GiB host memory.

Transient loopback tunnels exposed clone agent 22264 and ingress 36725 for qualification. Recovered worker credentials passed strict mTLS and reconciled retained containers. Native `f9459debe13d` and ingress `8c8c6ec4973b` were restored rather than recreated. Fixed guest operator readback matched live expiry/runtime/custody/agent-key data exactly. Recovered CA, worker and publisher keys matched retained live values without printing their contents. A read-only native account query returned zero accounts; this qualifies the current empty pilot, not populated-account recovery.

`qualify-restored-public-pds.mjs` uses only recovered public edge trust and fixed loopback 36725. It passed native health 0.4.5036, advertised service DID, exact OAuth metadata, zero repositories, closed XRPC/OAuth signup and WebSocket 101. The first Node check rejected the overlapping trust bundle even though the served certificate matched its newest entry. The harness now selects the newest captured leaf, verifies its fixed public-key digest, and keeps normal hostname/chain verification. The server certificate serial matched that captured leaf. No verification bypass or live trust change was used. The probe also passed when run with the recovered pinned Node binary.

Clone outbound DNS and direct-IP TLS probes to the PLC directory failed; the same read succeeded from the live guest. This verifies network isolation alongside QEMU's fixed launch policy. No cloned operator heartbeat, account/DID publication, external enrollment or public route was activated.

Both clone tunnels were stopped before clean guest shutdown; the restore unit is inactive and unenabled. `restore-qualified.json` on Kvant and `kvant-public-pds-20261012-operated.restore-qualified.json` on Dozor record authenticated independent restore, retained keys/runtime and stopped fixture. Production pilot/public checks and original NUC/Dozor health remained successful; private staging/Kuznya retained PIDs 1994020/930757. Current renewal and publisher inspections are healthy.

## Verification, continuation and limits

All 44 homelab tests passed, including three new archive-scope/type/duplicate cases. The full pinned smoke gate exited zero with 44 Node checks, frontend build, Go vet and race-enabled packages. Disposable PostgreSQL is stopped with its volume retained. New fixture scripts passed syntax/runtime checks; hosted CI remains separate.

To repeat inspection, retain the fixed restricted launcher and stopped private disk. Never run captured host units or publisher credentials against live paths as part of clone qualification. Stop clone tunnels before its guest. Preserve independent ciphertext/key custody and the prior checkpoint. An actual cutover requires fresh source/consumer inspection and authorization; this procedure does not activate a second publicly reachable identity.

The archive also snapshots current external public helper/trust/rule files, but restoration of Almaz/Dozor globals and live authorization/config is not exercised. Public host-loss cutover, physical Kvant reboot, naturally due renewal, populated restore, account/federation qualification and production management provisioning remain separate gates.
