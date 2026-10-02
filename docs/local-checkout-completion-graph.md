# Current checkout and subscription graph evidence

Status: implemented and locally verified on October 1, 2026. Ownership mapping and commercial entitlement remain open.

The local fixture adapter can now re-read a completed session and its expected subscription through the same pinned SDK client, authority and credential. The signed subscription claim must match the exact stored session before a subscription GET is allowed. The session read retains the earlier original customer/reference/expiry, fixed returns and configured single Price checks.

The second GET verifies first-class subscription/customer IDs, test mode, current known status, one non-paginated subscription item belonging to that subscription, one configured non-live Price, quantity one and a positive item period end. It returns a small session/subscription projection. Email, metadata, navigation and payment details are absent. Both reads share an eight-second deadline and make no creation call.

Incomplete, expired-incomplete, trialing, active, past-due, canceled, unpaid and paused are retained as evidence rather than entitlement decisions. An unpaid completed session stays unpaid, even when the subscription is active. No organization, binding, observation, hosting allowance or native action is selected or written. Commercial policy and serialized original/current owner mapping are required before use.

Focused races passed for exact SDK method/path/version, fixed line-item expansion, no idempotency header, all known states and unpaid evidence. Foreign claims stop before the subscription read. Malformed/live/deleted/foreign customer and subscription identities, pagination, multiple/nil/foreign items, Price object/identity/mode, quantity, period, redirects and provider failures return no graph. Invalid inputs/canceled contexts make no network request. New graph/state functions reached 100% focused coverage. The full snapshot-opt-in smoke gate passed, including legacy reconciliation and actual registry replay. The legacy reconciler now shares the same status allowlist without changing its accepted states.

This library change adds no schema, queue processing, HTTP caller or running configuration. Normal schema-33 readiness stays green, billing disabled, source work/native/custody matching. Optional golangci-lint remains unavailable. Private evidence: `.local/completion-graph-focused.log`, `.local/completion-graph-cover.out`, `.local/completion-graph-smoke.log` and `.local/completion-graph-source-after.json`.

Stripe documents the first-class subscription GET and subscription-item fields in its [subscription retrieval reference](https://docs.stripe.com/api/subscriptions/retrieve). See [signed completion intake](local-checkout-completion-inbox.md) and [registry replay](local-checkout-registry-recovery.md).

Original/current owner and retained resource mapping, atomic audit/observation, commercial hosting policy, interactive actual-authority browser, expanded positive-state responsive coverage, Stripe sandbox, independent backup custody and live qualification remain open.
