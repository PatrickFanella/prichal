# Local encrypted PDS recovery

This drill uses the fixed, isolated account qualification stack. Run it after a successful account journey:

```sh
./scripts/pds-local.sh qualify-account
./scripts/pds-local.sh qualify-recovery
```

It holds the account-qualification lock and temporarily stops only the `pds-ass-runtime` ingress, PDS and protocol fixture. Management PostgreSQL, the API, managed agent instances and unrelated containers keep running. Recovery uses fresh volumes, networks and containers under a unique `pds-ass-recovery-UUID` project. The original containers and volumes remain intact.

## Capture and authentication

Before capture, the drill writes a bounded fixture marker and inventories every local repository and referenced blob. Upstream libraries validate repository signatures and CAR hashes; each referenced blob is fetched and checked against its content CID and declared size. Inventory contains identifiers, hashes and counts rather than record content.

After all source writers stop, it copies the complete data, blob and protocol volume trees, including SQLite journal companions, into new staging volumes. The pinned runtime's [SQLite backup API](https://www.sqlite.org/backup.html) produces portable database copies there. It checks integrity and normalizes journal mode in the staging copy. Source databases and journals are preserved. File hashes then cover every captured database, blob, identity/mail fixture, protected configuration and required fixture script.

The backup uses AES-256-GCM with a fresh 12-byte nonce, a 16-byte authentication tag and authenticated version/snapshot/scope metadata. It follows Node's [authenticated encryption API](https://nodejs.org/docs/latest-v24.x/api/crypto.html). Each drill creates a separate private 32-byte recovery key outside the bundle directory. Decryption authenticates before publishing plaintext at the requested output path. Existing output files are never replaced. Tests reject wrong keys, changed metadata, modified ciphertext/tags, truncated files and unsafe key files.

Generated work files and restored configuration contain private runtime material. They stay in owner-only ignored `.local/recovery-work/`. Encrypted bundles stay in `.local/recovery-bundles/`; their keys stay in `.local/recovery-keys/`. Recovery keys do not enter API configuration, jobs, audit records or browser responses. Recovery reports contain only a key reference. Losing the recovery key makes that encrypted bundle unusable. Keep the original copy until a separately protected recovery copy is verified.

## Restore and fencing

The drill decrypts its authenticated bundle into a private staging file, extracts into fresh volumes and compares the full file/database inventory before any restored writer starts. The original writers must still be stopped. A separate Compose project then takes over the same local hostname and ports using protected restored configuration and the pinned images.

Verification compares all repository/record/referenced-media counts and content inventories, authenticates the original account, checks confirmed email and public identity, and writes a new signed record with a matching WebSocket event. The recovered PLC signer authorizes a temporary rotation in the restored directory; the new key signs the rollback to the original rotation set. These operations contact only the isolated directory and never change a public identity.

The finalizer stops every restored writer before restarting the original stack. A failed restored stop leaves the original stopped for operator review, preventing two active writers at the same endpoint. A successful drill retains stopped restored containers, fresh and staging volumes, the encrypted bundle, key and private report. Its restored write remains in that retained recovery copy; the source marker remains in the original.

A hard-killed drill can leave its exclusive lock and interrupted runtime state. Inspect the original and restored project inventories before resuming either writer. Preserve the report, snapshots and volumes for review. Remove only the empty generated lock after confirming its previous process is absent.

## Scope and measured limits

The final local drill captured 38 files, 13 SQLite databases, 9 repositories, 15 signed records and 8 referenced blobs, totaling about 1.45 MB. It recovered the designated source marker with no observed marker loss, verified the recovered PLC signer and completed local recovery in 8.43 seconds. The source marker preceded capture by 1.64 seconds. This measures a small cold localhost drill; it is not a scheduled RPO or production RTO guarantee.

The current local envelope is limited to 512 MiB and inventory to 256 MiB, 64 MiB per file, 32 repositories and 10,000 records/files. Production backup needs a streaming storage design and workload qualification. Modern blob references are covered here; legacy account/migration fixtures remain later work.

The key and bundle directories are separate on the same workstation. Independent recovery storage, separate access roles, retention schedules, encryption/KMS custody, automatic recovery alerts and recovery on a fresh hosting node remain live qualification gates. Management PostgreSQL, enrolled-node private keys, the agent CA and managed TLS identities need their own protected recovery inventories. This PDS drill does not close those gates or prove public DNS/TLS, external mail delivery or downstream federation.
