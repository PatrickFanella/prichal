# Local checkout completion inbox

Status: implemented and locally verified on October 1, 2026. Ownership mapping, current subscription observation and paid entitlement remain open.

Schema 33 stores only signed event identity/type/time, the raw-body digest, first-class test session/customer/subscription IDs and a request UUID claim. The claim has no foreign key to an organization or checkout request: an unknown claim is retained pending and cannot choose custody. Raw content, metadata, email, payment details and navigation are absent. These rows grant no access.

The separate `POST /api/billing/checkout-webhook` route requires explicit local session-fixture configuration and a constructed adapter. Enabling only the existing subscription inbox cannot enable it. Normal configuration returns 503. The route authenticates the bounded raw body, rejects browser Origin/Cookie headers including empty values, and acknowledges supported notifications only after the projection and one River job commit together. Verified unsupported types are ignored. The completion worker snoozes; mapping and fulfillment are deliberately not wired yet.

Concurrent duplicates preserve one row/job. Delivery metadata may change while immutable event/session/customer/subscription/reference identity must match. Both inboxes serialize the event identifier with a shared transaction lock, so conflicting cross-type identities cannot be accepted concurrently. Recovery is checked before intake, after lock/insert waits and before commit. A held duplicate receives no acknowledgement; an insertion or job failure leaves no partial work.

Focused races passed for duplicate and cross-inbox concurrency, altered immutable identity, queue rollback, private-content exclusion, independent HTTP gating, unsupported/oversized input and held/missing recovery. Late holds during the event lock and a blocked insert rolled back without a job. The schema-32 upgrade test preserved prior events/jobs and accepted new notifications only after migration. Full smoke passed with populated checkout snapshot opt-in. Snapshot reports now use the supported schema version rather than a hardcoded number.

With project management writers paused, the actual source upgrade preserved all fifty-six preexisting management tables apart from the version marker; the new inbox was empty. Fresh recovery `f80db243-1d04-4915-8127-3812183f77a7` verified fifty-six tables/sequences, revoked copied authentication, refused held worker startup and reconciled/replayed without changing source management rows/sequences, copied default jobs, native signed data/media, containers or credentials/TLS. The retained copy remains available in ignored `.local`.

Normal local API/worker/monitor resumed. Readiness reports schema 33 and the disabled completion route returns 503. Source checkout requests/audits/notifications remain zero; legacy events/bindings/observations remain eight/one/one and jobs seventy-nine. Source work/native/custody inventories match. No actual Stripe resource, charge, paid entitlement or native action occurred. Optional golangci-lint remains unavailable.

Private evidence: `.local/completion-inbox-focused.log`, `.local/completion-inbox-smoke.log`, upgrade inventories, recovery/replay logs and source inventory/count reports. See [completion identity](local-checkout-completion-evidence.md) and [billing recovery fences](local-billing-recovery-fences.md).

Original/current owner mapping, retained first-class IDs, current authoritative subscription/catalog verification, commercial entitlement policy, actual restored-registry replay, interactive authority browser, remaining responsive widths, Stripe sandbox and live qualification remain required work.
