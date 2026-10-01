# Managed tenant cold recovery

This operator-only drill extends the isolated protocol recovery checks to a populated managed UUID deployment. It accepts a private local owner-journey directory and validates owner scope, the exact TLS endpoint, pinned images, protected per-instance credentials, native volumes and the fixed development identity fixture before stopping writers.

```sh
node scripts/qualify-managed-recovery.mjs .local/owner-enrollment/JOURNEY_UUID alice
```

It captures complete native data and blobs with SQLite companions, per-instance runtime credentials, the TLS identity, ingress policy and the closed registration lease. It also captures the shared local PLC/mail fixture, so this bundle contains development identities and captured mail beyond the selected tenant. It is private operator material and must never be offered as a customer export.

The source PDS, ingress, guard and local protocol fixture stop during capture. Source volume users must be fenced. SQLite backup normalization operates on new staging volumes only; source databases remain untouched. A file/hash inventory and integrity checks cover the normalized copies. The encrypted bundle uses a fresh private key kept outside it, authenticates metadata and bytes, and publishes no unauthenticated plaintext.

Fresh volumes and a fresh private restoration network recover the same account and endpoint. The restored ingress uses the captured TLS identity on the original loopback port while the original ingress is stopped. The guard receives only its dedicated closed-lease directory, with no access to other protected host files. Native account login, confirmed email, signed repository records and referenced media must match. A new restored write must produce a verified repository signature. The recovered PLC signer adds and rolls back a temporary key only in the restored isolated directory.

Restored writers stop and their volumes are checked for active users before source writers resume. The source directory identity, source signed records and container remain unchanged. Keys, encrypted bundles, private working metadata and stopped restored/staging resources remain for review under `.local/`. No source volumes are deleted. Cleanup also attempts fencing and restoration on interruption; inspect a failed run before retrying.

The final local drill captured 15 files, 5 SQLite databases and 620,897 bytes, preserving one signed repository with 4 records and one referenced blob. It verified a fresh signed write and recovered identity rotation, then resumed the original with its source identity unchanged. The measured cold drill took 7.087 seconds. This small localhost result is not a production RTO or scheduled RPO guarantee.

The management database, enrollment encryption key, agent/CA credentials and off-machine backup custody have separate recovery requirements. The shared local identity fixture, foreground scheduling, same-machine keys and tiny dataset do not qualify production recovery, retention or independent storage. Issues #11 and #12 remain open for those gates.
