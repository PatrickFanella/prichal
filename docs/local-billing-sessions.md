# Local Checkout and Portal adapter

Status: transport adapter implemented and locally verified on October 1, 2026. Durable owner request handling, browser routes and actual Stripe qualification remain open.

The adapter uses the existing pinned instance SDK and private fixture key with a numeric loopback HTTP authority. Both billing and return origins must be canonical loopback origins. An explicit fixture Price and Portal configuration are required. Production and external billing authorities remain refused.

Checkout uses hosted subscription mode, one configured Price with quantity one, an exact request reference and fixed return/cancel paths. It omits payment-method restrictions and automatic tax. Checkout and Portal have separate idempotency namespaces; the same request retains identical parameters and its integration label. Portal uses the exact customer and explicit configuration. Both calls honor cancellation and an eight-second deadline, without redirect following, SDK retries or provider logging.

Responses must match the expected customer, resource shape, mode and request reference. Live, expired, terminal or mismatched Checkout sessions are rejected; Portal configuration and creation time must match the permitted fixture shape. Navigation URLs must identify the exact returned session under its fixture authority. External URLs, credentials, encoded paths, query/fragment additions and unexpected paths are refused. Errors return no session capability and disclose no provider detail.

This adapter does not establish organization ownership, bind a new customer, reconcile payment, grant entitlement, alter a pilot grant or cancel hosting. Its session URL is an ephemeral capability and must not be logged or stored in audit evidence. The future durable store must derive customer identity from a retained owner-authorized mapping before calling it. The adapter has no HTTP registration or production caller.

## Verification

Disposable HTTP fixtures verify exact SDK parameters and version, identical retries and separate namespaces, malformed input refusal before network access, cancellation, untrusted response fields, invalid navigation, provider errors and redirect refusal. Focused race tests and full local smoke passed. Session functions have 88.9–100% focused coverage; the remainder of the billing package is covered by its separate existing integration suite. No actual Stripe resource or charge was created.

Ordinary source billing remains disabled on schema 30. Source work/native/custody and seven cancelled choices/fourteen audits matched the baseline. No runtime restart or source mapping was needed. Private results remain in ignored `.local/billing-session-final-tests.log`, `.local/billing-session-smoke.log` and `.local/billing-session-source-after.json`. Optional golangci-lint remains unavailable.

Next work includes durable original-owner checkout intent and customer mapping, receipt reads and browser review, authoritative Product/Price validation, completion-event mapping into required webhook reconciliation and explicit entitlement policy. Actual isolated Stripe sandbox resources, commercial plans/grace, Portal behavior, tax registrations, independent recovery and live billing remain unqualified.

The request shape follows Stripe's [Checkout Session API](https://docs.stripe.com/api/checkout/sessions/create) and [Portal Session API](https://docs.stripe.com/api/customer_portal/sessions/create). The fixture URLs are deliberately separate from actual Stripe navigation. See [existing required webhook inbox](local-billing-inbox.md).
