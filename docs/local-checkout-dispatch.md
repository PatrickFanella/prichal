# Local durable checkout dispatch

Status: implemented and locally verified on October 1, 2026. Owner HTTP/browser routes, hosted-session recovery and payment fulfillment remain open.

An explicitly reviewed original-owner request queues one fixture checkout job and audit atomically. Preparing a plan alone still queues nothing. Concurrent starts retain one original job. Existing billing, pending handover, changed configuration or uncertain recovery prevents dispatch. Read receipts continue to report no payment entitlement.

Before any remote call, the worker commits an original dispatch reservation. Customer creation sends only a fixed fixture description and its separate idempotency key. It sends no email, owner identity, organization name, metadata or payment method. The verified first-class customer ID is checkpointed before Checkout can run. Checkout uses the original request UUID and that exact customer; its ID/expiry are checkpointed separately. No hosted navigation URL is persisted or audited.

Queueing binds the canonical fixture authority and a SHA-256 fingerprint of the credential used by the actual SDK client. Price and return origin are also fixed. A different authority, credential or configuration cannot move an unresolved request into another namespace. The fingerprint records scope without storing the key. Fixture key rotation requires separate requalification; production account/credential rotation is not qualified.

Unknown customer/session results retry only their original step/key while its durable reservation is less than twenty-three hours old. Confirmed steps do not run again. Older unresolved requests enter manual review without another remote call or discarded IDs. Clock anomalies fail closed. This conservative fixture cutoff avoids reuse after the provider's [idempotency retention window](https://docs.stripe.com/api/idempotent_requests).

Current original ownership is locked through each remote step. Recovery is checked after waits, immediately before RPC and before checkpoint commit. A hold during an already-authorized response preserves the pending checkpoint and stops later steps; it cannot undo an RPC already begun. Management writers must still be quiesced for snapshots. Held/missing recovery and disabled or changed authority leave worker jobs pending.

## Configuration boundary

The existing signed notification fixture does not enable session creation. The separate `PDS_BILLING_SESSIONS_MODE=fixture` gate requires explicit local mode, enabled fixture billing, private key, canonical numeric-loopback authority/return origin, `PDS_BILLING_PRICE_ID` and `PDS_BILLING_PORTAL_CONFIGURATION_ID`. Empty/disabled mode recognizes queued jobs but snoozes them without authority. External/live billing remains refused.

The worker is registered for durable checkout work. No owner HTTP route submits it yet, and ordinary source session mode remains disabled. No actual Stripe resource, charge, entitlement, native action or subscription cancellation was created.

## Verification

Full local smoke and focused race checks passed. Disposable fixtures cover concurrent queue/replay, unchecked submission, queue/audit/reservation rollback, lost remote responses, checkpoint loss, exact bounded retry, terminal suppression, expired unknown work, changed owner/Price/authority/credential, future clock and customer collision across organizations. Recovery tests cover pre-dispatch holds/missing ledger, holds during custody waits, queue audit waits and already-authorized responses. Missing-ledger worker results snooze with unchanged work.

A combined disposable HTTP/SQL test exercised the pinned instance SDK through both customer and Checkout creation, verified the retained authority/fingerprint and separate keys, then suppressed all repeat calls after confirmation. The customer adapter follows Stripe's [Customer API](https://docs.stripe.com/api/customers/create). SDK responses and redirects fail closed under the existing [session transport](local-billing-sessions.md).

Schema-31 reconstruction verified unchanged original preparation/audit values, default owner attribution and no invented remote work after applying schema 32 twice. The actual upgrade preserved all fifty-six management tables apart from the intended version marker. Source checkout request/audit tables remain empty, with seventy-nine original jobs.

Fresh recovery `12fd0de1-24fd-4404-8ddf-e7912b275834` completed capture at 01:13:57Z on October 2. Fifty-five tables/sequences matched, copied authentication was revoked and held worker startup was refused. Reconciliation/replay preserved source tables/sequences, default jobs, native containers, signed records/media, credentials/TLS and admission state. Normal API/worker/monitor resumed ready on schema 32 with ordinary permissions off and admissions open. Source work/native/custody and seven cancelled choices/fourteen audits matched.

Private evidence remains in ignored `.local/checkout-dispatch-smoke.log`, `.local/checkout-dispatch-focused.log`, `.local/checkout-dispatch-upgrade-before.json`, `.local/checkout-dispatch-upgrade-after.json`, `.local/checkout-dispatch-source-after.json` and recovery reports. Retained tenants, volumes, exports and recovery copies remain preserved. Optional golangci-lint remains unavailable.

Populated checkout restore, actual authorized owner dispatch, read-only hosted-session recovery, browser review, completion-event mapping into required reconciliation, authoritative catalog/entitlement/grace policy, actual isolated sandbox resources, tax registrations and live billing remain unqualified. See [preparation](local-checkout-preparation.md) and [required billing inbox](local-billing-inbox.md).
