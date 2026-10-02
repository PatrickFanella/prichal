# Local durable checkout preparation

Status: schema 31 and owner-scoped preparation are locally verified on October 1, 2026. Checkout dispatch, customer mapping and owner browser routes remain open.

Preparation records the original owner, organization, configured fixture Price, fixed return origin and exact request key. One unresolved preparation is permitted per organization. The receipt always reports `canPay=false`; it contains no navigation capability or customer/payment content. Preparation creates no remote customer, session, River job, entitlement or native action.

An explicit local fixture adapter is required. Unchecked or invalid requests are refused. Organization ownership is locked through review, insertion and audit. Identical retries preserve the original receipt and add no audit. A different key cannot replace an unresolved request. Changed Price/return configuration fails closed; neither a former owner nor a new owner can adopt the original request.

Read-only lookup by request UUID or original request key recovers a lost response. Reads require both original and current custody under a shared organization lock. Held or missing recovery state blocks preparation, retries and receipt disclosure. Recovery is rechecked after lock waits, after materialization and before committing the preparation audit. Audit failure or a hold introduced during audit wait rolls back both rows.

Existing billing bindings and pending ownership handovers prevent new preparation. Prepared checkout blocks both a new empty-organization handover offer and acceptance of an earlier offer. No pending request is silently transferred or discarded.

## Verification

Disposable database race tests verify concurrent original-key replay, exact receipt/key recovery, changed configuration and competing-key refusal, unchecked/invalid requests, foreign and changed ownership, audit rollback, existing billing/handover refusal, held/missing recovery, and recovery introduced during organization, read-table and audit lock waits. Preparation dispatch count and queue count remain zero.

Migration tests reconstruct schema 30 with retained billing events, customer binding and queued work. Applying schema 31 twice preserves those complete rows and creates empty preparation tables. The initial full gate caught two older enrollment migration fixtures that retained the new tables while resetting their version. Their reconstructed schemas were corrected; the final full local smoke gate passed.

The actual source upgrade preserved all fifty-four existing management tables apart from the intended schema-version change. Both new tables and claim tables are empty. Fresh recovery `eae20876-6790-4044-a922-9b34715ec15f` completed capture at 00:37:54Z on October 2, verified fifty-five tables and sequences, revoked 106 copied management sessions and other copied authorization, and refused held worker startup. Reconciliation/replay preserved all source management tables, sequences, default jobs, native containers, signed records/media and custody.

Normal API/worker/monitor resumed with ordinary billing/operator/collaboration permissions off and all admissions open. Actual browser readiness returned schema 31 with fixtures and retained operator keys absent. Source work/native/custody and seven cancelled choices/fourteen audits matched the baseline. Retained tenants, containers, volumes, export archives and recovery copies remain preserved.

Private evidence is in ignored `.local/checkout-intent-smoke.log`, `.local/checkout-upgrade-before.json`, `.local/checkout-upgrade-after.json`, `.local/checkout-intent-source-after.json` and the recovery capture/replay reports. These source snapshots contain empty preparation tables; populated checkout recovery is not claimed. Optional golangci-lint remains unavailable.

Next work connects durable customer mapping, bounded original-key remote dispatch and read-only hosted-session recovery to this preparation. Owner HTTP/browser review, required completion-event reconciliation, authoritative catalog validation, actual isolated Stripe sandbox resources, entitlement/grace/tax policy and live billing remain unqualified. See [session transport](local-billing-sessions.md) and [required billing inbox](local-billing-inbox.md).
