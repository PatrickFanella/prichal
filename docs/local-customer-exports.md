# Local customer exports

The owner workspace can request a portable snapshot of a ready local native PDS. Each tar archive contains `manifest.json`, a signed `repo.car` and referenced media under `blobs/{CID}`. Version 2 adds `preferences.json` for bounded private Bluesky settings. The manifest includes public signing identity and content hashes. See [export and exit](export-and-exit.md) for supported state and migration preparation. Native SQLite databases, account passwords, email, JWT secrets, TLS keys, PLC rotation keys and admission state are excluded.

The API checks current organization ownership and CSRF protection before retaining a stable UUID request and River job in one transaction. Export progress, lists and downloads also require the owner session. Foreign owners receive 404. CLI credentials have no export scope. The worker selects the registered node and verified native DID; the restricted mTLS agent runs an embedded helper against the sole expected actor. The helper verifies CAR signatures and media CIDs before returning content. The API never receives Docker access or native credentials.

Capture serializes with managed lifecycle actions. An account hold allows export through the private native admin route while public login stays denied. A stopped service leaves capture queued until it resumes. Active native writes may continue: the archive captures one signed repository version, and missing referenced media fails capture rather than publishing a partial bundle.

## Local limits and custody

CAR and aggregate media are each bounded to 8 MiB. A blob is at most 5 MiB; there are at most 1,000 referenced blobs and 10,000 signed records. The complete archive is at most 20 MiB. Private preference content is bounded to 1 MiB and 1,000 values. Each deployment permits three retained requests, including failed or expired requests. Stable retries retrieve the original operation before the allowance check.

`./scripts/dev.sh` initializes a private `.local/exports/` directory and separate 32-byte `.local/export.key`. Stored archives use AES-256-GCM bound to export, deployment and native DID. Atomic publication preserves an existing authenticated artifact; replay after a lost management checkpoint reuses its original content and capture time. A corrupt existing artifact is retained and fails verification rather than being replaced.

Downloads authenticate the whole archive and its database byte/hash checkpoint before writing a response. They use a fixed UUID filename, `no-store` and `nosniff`. The download window lasts 24 hours from capture. Expiry closes access without deleting the source or stored artifact. Keys and artifact paths never appear in customer progress responses. Direct startup disables exports unless the private local storage configuration is supplied; nonlocal mode rejects it.

The browser retains its stable request key in session storage before submission and offers an explicit retry after an uncertain response. Reloaded requests do not submit automatically. Owner-authorized lists recover completed requests. Browser rendering, responsive behavior, accessibility and persistence remain unqualified because the attached T3 preview snapshot tool failed.

## Qualification

Start the separate local PostgreSQL, issuer, API, mTLS agent, worker and web workspace using the existing local setup. The account fixtures must already be populated; this drill creates no new PDS or account.

```sh
./scripts/dev.sh smoke
node scripts/qualify-customer-exports.mjs .local/owner-enrollment/BOB-JOURNEY-UUID
node scripts/qualify-export-custody.mjs .local/customer-exports/REPORT-UUID/report.json
```

The first drill uses the disposable Alice/Bob owners and Bob's exact UUID.localhost PDS. It downloads normal and held-account exports, tests CSRF and owner isolation, then separately resumes the held account through an audited action. The independent verifier uses the pinned native packages offline to check signed records, referenced media and every archive member. It compares source native inventory, container identities and protected file hashes before and after. Private archives and reports remain in `.local/`.

The custody drill backs up only those encrypted artifacts, retains the export and envelope keys separately from the bundle, restores fresh copies, rejects wrong keys, tampering and wrong scopes, and independently verifies the recovered portable content. The existing management restore/replay drill separately checks all schema 21 tables and sequences, copied authentication revocation and host reconciliation. All copies remain retained.

On 2026-10-01, qualification `dd82134d-ef81-4c17-b636-940c86cc1aea` downloaded two 6,656-byte archives, each with six signed records and one media blob. Account generation 23 held public login; generation 24 resumed it. Custody recovery `9580a614-5ff6-4eca-af5f-22ca99eaa26e` verified fresh copies. See [local progress](progress/local-m1-alpha.md) for management recovery and current verification evidence.

## Remaining gates

Independent storage and key custody, scheduled capture, retention and deletion policy, larger streamed exports, commercial cancellation, notifications, live delivery and actual account migration remain open. This archive supports portable data access; importing it into another PDS and changing identity routing are separate work. Local exports do not qualify public hosting or complete the full milestone.
