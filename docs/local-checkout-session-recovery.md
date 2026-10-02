# Local checkout session recovery

Status: implemented and locally verified on October 1, 2026. Owner HTTP/browser routes remain open. Ordinary billing sessions remain disabled.

An original current owner can recover only a confirmed session checkpoint from the same fixture authority, credential fingerprint, configured Price and return origin. Ownership and request locks stay held through the read. Held/missing recovery, existing billing, unfinished checkout, foreign custody or changed configuration prevents navigation.

The adapter uses the pinned SDK's [Checkout retrieval API](https://docs.stripe.com/api/checkout/sessions/retrieve) with expanded line items. It verifies the exact session ID, mapped customer, original request reference, saved expiry, subscription/hosted mode, fixed return URLs and one configured Price at quantity one. Only a verified open session can return a temporary numeric-loopback URL. Complete/expired sessions return no URL and grant no entitlement. An expired local checkpoint returns its expiry without any remote call.

Recovery never creates, renews, expires or cancels a remote resource, and writes no request, audit or job. Navigation URLs stay transient. A recovery hold during the GET discards its response. A hold cannot undo a read that already began; snapshots still require writer quiescence.

Focused race tests passed for the pinned GET/expansion/version, unchanged repeated reads, closed/expired sessions, input/cancellation before network, foreign/live/malformed responses, redirect/provider failure, wrong customer/reference/Price/quantity/expiry, current/original custody and configuration changes. Holds before the read, during organization/request lock waits and during the remote response return no navigation and preserve complete checkout/job/audit ledgers. Function coverage was 100% for retrieval and 88.2% for owner recovery in the focused run.

The full local smoke gate passed. The running API remains ready on schema 32. Source work/custody inventories, seven cancelled choices and fourteen audits match the baseline. This library addition changes no schema or runtime configuration and creates no source checkout or actual Stripe resource. Optional golangci-lint remains unavailable.

Private evidence remains in ignored `.local/checkout-session-recovery-focused.log`, `.local/checkout-session-recovery-cover.out`, `.local/checkout-session-recovery-smoke.log` and `.local/checkout-session-recovery-source-after.json`. See [durable dispatch](local-checkout-dispatch.md) for checkpoints and the unresolved retry cutoff.

Owner HTTP/browser review, populated checkout restore, signed checkout-completion mapping into mandatory authoritative reconciliation, commercial entitlement/grace policy, actual isolated sandbox and live qualification remain open.
