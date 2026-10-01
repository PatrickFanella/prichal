# Local populated claim recovery

Status: disposable snapshot qualification passed on October 1, 2026. Controlled public DNS and independent recovery remain unqualified.

`TestPopulatedClaimSnapshotRecovery` creates isolated databases using the existing guarded local test allocator. It requires an explicit opt-in, numeric loopback PostgreSQL on port 35433 and the fixed project database container on a local Docker socket. The source is a generated `pds_smoke_` fixture, never the running management database. The restore is a generated `pds_recovery_` database. No native tenant or host credentials are copied.

The fixture uses injected TXT matches to create three claims and four claim audits. It retains a released receipt, an active reservation and a logically expired reservation, plus a competing saved choice. A full PostgreSQL custom-format dump is restored into the empty recovery database. Choice and claim rows and their complete audits match before and after.

Applying the real recovery hold refuses original-owner reads, exact reservation retries and cancellation release before querying DNS. The ledger remains unchanged. Reconciliation verifies the fixture has no registered hosts or placements and obtains no host authority. After reconciliation, repeated exact retries return reserved/released/expired receipt authority without DNS or new audits. A competing verified owner is still refused by the restored active reservation. Foreign-owner reads remain denied. Every receipt keeps `canActivate=false`; the source fixture ledger remains unchanged.

## Run locally

Start the separate local database with `./scripts/dev.sh db`. With the existing ignored local environment loaded, run from the repository root:

```sh
set -a
source .local/local.env
set +a
cd backend
PDS_TEST_ADMIN_URL="$PDS_DATABASE_URL" PDS_TEST_LOCAL_CLAIM_SNAPSHOT=true \
  go test -race -count=1 ./internal/endpointplan -run '^TestPopulatedClaimSnapshotRecovery$'
```

The optional test skips during the normal smoke gate because CI has a different PostgreSQL layout. It was separately run and passed against the fixed local PostgreSQL. The full `./scripts/dev.sh smoke` gate also passed. Safe reports are retained under ignored private `.local/claim-recovery-*/`; fixture databases are removed by the test allocator, and the raw dump stays in process memory. No retained customer database, export or recovery copy is removed. The ordinary source inventory remained seven cancelled choices/fourteen audits/zero pending choices with matching work/native/custody hashes; source claims remain absent.

## Remaining gates

This proves populated synthetic claim snapshot and hold/reconciliation behavior. It does not establish public DNS ownership, expiry during a long outage, independent backup custody, operator recovery on a different host, populated native-placement reconciliation for these claims, routing/TLS or customer hosting activation. The earlier schema-30 management drill separately covers actual retained native placements with empty claim tables.
