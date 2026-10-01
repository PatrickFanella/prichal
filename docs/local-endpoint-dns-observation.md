# Observing a saved endpoint TXT challenge

The local owner API can check DNS for a saved custom-address challenge without recording a verified claim. `POST /api/organizations/{id}/endpoint-choices/{choice}/dns-observation` accepts only an empty JSON object and requires the current original owner, browser Origin/CSRF and local mode. Production and customer CLI authority remain unavailable.

The resolver is selected by the service, never the customer. The API uses the system DNS resolver and queries only `_pdsaas.<validated saved hostname>`. It fetches no customer URL. Lookup has a four-second context within the eight-second request deadline. Results contain fixed state/message, choice/hostname references, one database check time and false claim/activation flags; raw TXT answers, proof values and DNS errors are omitted.

Cancelled and expired choices send no query. Local `.test` choices return `local_namespace` without a query and cannot establish public ownership. For public names, at most 16 TXT records and 4096 aggregate bytes are accepted. An exact saved challenge may coexist with other scoped challenges; duplicates do not create a new claim. Missing/mismatched answers remain pending, oversized answers are refused and resolver failure stays unavailable.

After lookup, the query rechecks current ownership, original actor, cancellation, proof/payload identity and expiry. Cancellation/expiry clears a transient match; ownership loss denies the result and changed proof conflicts. Expiry and the reported check time use the same PostgreSQL statement timestamp. No observation updates the choice, writes an audit or queues work.

`challenge_match` means the resolver returned the exact unexpired challenge at that check. It grants no exclusive domain reservation or activation authority. A separate [local exclusive-claim workflow](local-exclusive-endpoint-claims.md) now serializes competing reservations and supports release/expiry. Positive public claims, resolver trust and routing/TLS/provisioning remain unqualified. Observation itself still grants no reservation. Existing native endpoints remain untouched.

## Verification

Full smoke passed, including Node checks, frontend build and Go vet/race/disposable PostgreSQL tests. Focused package race/coverage verification passed with 84.5% statement coverage. Tests cover exact/duplicate/other-choice answers, multiple scoped challenges, mismatches, NXDOMAIN, fixed private-error projection, answer count/byte bounds and resolver deadlines. Foreign/cancelled/expired/local/production/nil-resolver paths send no query. Blocking resolver tests change cancellation, expiry, ownership or challenge during lookup and refuse stale matches. Parent cancellation stops lookup. Strict body and real route authorization boundaries are covered. Optional golangci-lint remains unavailable.

Actual local drill `72c2f4bb-7acc-4fdc-be2f-98362ee0ef3d` completed at 19:24:00Z. A `.test` choice returned local-namespace guidance repeatedly without claiming ownership. Foreign scope, missing-CSRF, customer resolver settings and CLI authority were denied. Reads preserved choice and audit counts. Separate cancellation retained the receipt; the cancelled observation remained non-matching. One cancelled receipt/two audits were added, leaving six choices/twelve audits and zero pending choices. Management work, endpoints, admission policies and native repositories/media, containers and protected custody matched the baseline.

After making the database timestamp coherent, a fresh full smoke/coverage gate passed and the updated API reread the cancelled receipt at 19:26:27Z without claim/activation authority. Positive public DNS matching is verified only by injected resolver tests, not a controlled live domain. Browser observation response/focus/denial behavior and all five widths subsequently passed; shared fixture cleanup is complete. See [observation controls](local-endpoint-observation-controls.md). Controlled public DNS proof remains unqualified.

Schema remains 26. The retained 49-table recovery checkpoint predates the later browser/DNS-drill receipts; this read-only feature adds no schema or job kind. Local services retain fixture permissions disabled and both admission policies open. No native container, volume or recovery copy was removed. Verified exclusive claims, routing/TLS, activation and account migration remain open.

## Recovery fence, October 1

Standalone observations now inspect current owner scope and recovery state in a short transaction before DNS and again after lookup. A held or missing ledger refuses observation without DNS; a hold or lost ledger introduced during lookup discards the result. The transaction ends before DNS, so owner cancellation and proof changes remain possible during the bounded network lookup. No observation, choice or claim audit is written.

Four regression cases first failed: held/missing recovery before lookup and held/missing recovery during lookup all returned a match. After the fix, focused race checks and the full local smoke gate passed. HTTP checks return a generic 503 without proof data or a matching observation. Public DNS, persistent DNS request budgets and routing/activation remain open.
