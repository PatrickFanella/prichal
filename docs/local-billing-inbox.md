# Local billing event inbox

Schema 19 provides a signed event inbox and durable reconciliation jobs. It is a local fixture qualification for #18, not a payment integration. Observations never grant paid entitlement, change pilot grants, stop servers or delete data. Billing is disabled by default; only explicit local fixture mode with a numeric loopback authority is supported.

## Delivery and ownership

`POST /api/billing/webhook` verifies the original, bounded request body before parsing identifiers. Missing, invalid, expired or future signatures, live mode, account/context routing and incompatible API versions are rejected. Supported subscription events and subscription-backed invoice events queue reconciliation. Other event types and one-off invoices are acknowledged without work.

The inbox and River job commit together. Concurrent deliveries produce one event and one job. Duplicates compare event type, creation time, customer and subscription identifiers. Delivery metadata can change without creating work; a conflicting reconciliation identity is rejected. The first raw-body SHA-256 digest is retained, but raw invoices, customer metadata and payment content are not stored. See the [Stripe event object](https://docs.stripe.com/api/events/object).

A local operator binds an organization to exact fixture customer/subscription identifiers with an immutable, idempotent mapping. Email addresses and customer metadata cannot establish ownership. The worker locks the organization and reads current subscription state from the authority, so a late paid notification cannot overwrite a current canceled subscription. Unknown mappings, unavailable authority and recovery holds leave work pending. Failures roll back the observation and event checkpoint together.

The adapter uses an instance of `stripe-go/v87` pinned to v87.0.0 and API version `2026-09-30.endive`. Its fixture transport has bounded requests, no redirects, no network retries and quiet SDK logging. It rejects live responses and unsupported subscription item shapes. This exercises the SDK against our fake HTTP authority, not Stripe's sandbox.

## Run locally

Start the ordinary disposable PostgreSQL and local stack first. These commands create synthetic keys and fixture state only in ignored, private `.local/billing-fixture/`:

```sh
./scripts/billing-fixture-local.sh run
./scripts/billing-fixture-local.sh api
# Use the usual local agent mTLS environment when starting this worker:
./scripts/billing-fixture-local.sh worker
./scripts/billing-fixture-local.sh bind \
  --organization ORGANIZATION_UUID --customer cus_fixtureAlice \
  --subscription sub_fixtureAlice --key STABLE_UUID
node scripts/qualify-billing-inbox.mjs
```

Run the API and worker in separate terminals, replacing only this project's existing management processes. The qualifier requires the two populated local Alice/Bob tenants and retained private journey evidence. It changes fake subscription state, not native accounts or paid permissions. Key initialization preserves existing keys and bindings. Return to ordinary `dev.sh api` and `dev.sh worker` afterward; the webhook returns 503 when disabled.

## Evidence and remaining gates

The full local smoke gate passed Go build/vet/race tests, migrations, frontend build and Node checks. Coverage includes signature failure, unsupported invoice graphs, transactional queue failure, concurrent delivery, changed delivery metadata, unknown ownership, current-state reconciliation, serialized authority reads, private transport configuration and recovery holds.

The latest actual drill, `afc197b2-f48c-4328-a270-66ab42091cef`, completed at 2026-10-01T11:38:32Z. Four new events reconciled. Duplicate delivery, including changed `pending_webhooks`, produced one job. Changed event identity failed. An older paid notification retained the current canceled observation. Native resources, protected credential/TLS/admission files, deployment work and all eight pilot grant records remained unchanged. Across both drills the source retained one binding, eight events and one current observation.

Paid Checkout/Customer Portal, actual sandbox resources, restricted API keys, Product/Price ownership, subscription replacement, grace policy, risk/refund/dispute handling, periodic reconciliation, customer notification and tax configuration remain open. No actual Stripe resources or charges were created. Live billing remains closed. See [subscription webhooks](https://docs.stripe.com/billing/subscriptions/webhooks) and [signature verification](https://docs.stripe.com/webhooks).
