# Populated checkout snapshot recovery

Status: locally qualified on October 1, 2026 using allocator-owned disposable databases. Actual source checkout remains empty.

The optional `PDS_TEST_LOCAL_CHECKOUT_SNAPSHOT=true` qualification requires the already-running fixed `pds-ass-local-db-1`, project/service labels, a local Unix Docker socket and guarded numeric-loopback PostgreSQL. Both databases come from `testdb`; it refuses source database names. It does not copy source management rows or acquire native host execution authority.

The fixture prepares five organizations across prepared, queued, customer-ready, session-ready and manual-review states. Pending unknown dispatches carry an original reservation older than twenty-three hours; customer-ready work retains its confirmed first-class customer. A bounded custom-format pg_dump stays in memory and restores into a new allocator-owned recovery database. Five requests, seventeen audits and four original jobs match exactly before the hold.

Holding the copy revokes its copied authentication. Catalog, receipt, original-key recovery, preparation, dispatch and navigation are refused across every state. The checkout worker leaves work pending. Complete request/audit/job ledgers remain unchanged and both transport counters stay zero. The original fixture remains unchanged.

Reconciliation observes no hosts because this management fixture has none. It does not qualify any actual host or independent backup custody. Original receipts recover with unchanged keys, Price, custody, state and no payment capability. Terminal dispatch replay makes no RPC and changes no receipt or audit. Expired queued and customer-ready work enters manual review only in the reconciled copy, without remote calls, renewal, replacement session or discarded customer. The original fixture ledger stays unchanged.

Focused race qualification and the full smoke gate with explicit snapshot opt-in passed. Source work/native/custody and seven cancelled choices/fourteen audits match. Ordinary schema-32 API/worker settings remain unchanged and billing sessions stay disabled. No source checkout, actual billing resource, entitlement or native action occurred. Optional golangci-lint remains unavailable.

Run locally with `PDS_TEST_LOCAL_CHECKOUT_SNAPSHOT=true ./scripts/dev.sh smoke` after starting the separate local database. Default CI/local smoke skips this Docker-specific snapshot test and still exercises the actual Node/River/SDK fixture journey. The qualification deletes only its allocator-owned test databases and retains a safe 0600 report under ignored `.local/checkout-recovery-*/report.txt`. No snapshot or raw credential is logged or written to that report.

Private evidence remains in `.local/checkout-snapshot-focused.log`, `.local/checkout-snapshot-smoke.log`, `.local/checkout-snapshot-review.md` and `.local/checkout-snapshot-source-after.json`. See [actual synthetic authority journey](local-checkout-authority-fixture.md) and [source schema-32 recovery](local-checkout-dispatch.md).

Fresh unresolved replay against a restored actual provider registry, interactive actual-authority browser, remaining responsive widths, signed completion/subscription mapping into authoritative reconciliation, commercial policy, actual Stripe sandbox and live qualification remain open.
