# Billing recovery transaction fences

Status: implemented and locally verified on October 1, 2026. This changes fixture inbox/binding/reconciliation behavior, without paid entitlement or native action.

Signed notification intake now checks recovery in its transaction before insertion, after conflict/insert waits and before acknowledgement. A late hold rolls back the notification and queued work; duplicate delivery is not acknowledged under uncertainty. Immutable fixture binding checks recovery before and after organization waits and before commit. Binding and reconciliation have an eight-second total deadline.

Subscription reconciliation checks recovery after the event and organization locks, immediately before the provider read, after its response and before the final commit. Applied receipts also recheck after their lock wait. A hold during an observation write rolls back both observation and event consumption. A provider read already begun cannot be undone, but its response grants no retained observation while held. Snapshots still require writer quiescence.

The billing transaction check is shared with checkout preparation, dispatch, catalog and session recovery. Missing recovery state is classified unavailable, so workers snooze without consuming failure retries or discarding their jobs. Checkout behavior remains covered by the full existing suite.

Focused races passed for held/missing intake and duplicate acknowledgement, immutable binding replay, worker snooze, holds during event/organization/applied-receipt waits, a hold introduced by the provider response, a hold during observation upsert, binding custody wait and notification insertion rollback. Complete notification/binding/observation/job rows stay unchanged after refusal. Observation-write testing allows the already-authorized read and verifies no committed change.

The full local smoke gate passed. Only the verified project API and worker processes were restarted; monitor, agent, native tenants, issuer and browser stayed running. API readiness is green on schema 32. Source billing row hashes match across restart: eight events, one binding and one observation. Source checkout is empty, jobs remain seventy-nine, and work/native/custody plus seven cancelled choices/fourteen audits match. Ordinary billing sessions/inbox remain disabled. No source hold, actual billing resource, entitlement or native action occurred. Optional golangci-lint remains unavailable.

Private evidence remains in ignored `.local/billing-recovery-fences-focused.log`, `.local/billing-recovery-fences-smoke.log`, `.local/billing-recovery-fences-review.md`, `.local/billing-recovery-fences-source-before.sha256`, `.local/billing-recovery-fences-source-after.sha256`, `.local/billing-recovery-fences-source-counts.json` and `.local/billing-recovery-fences-source-after.json`.

Signed checkout-completion mapping and authoritative subscription/customer/catalog confirmation, commercial policy, actual isolated Stripe sandbox and live qualification remain open. See [billing inbox](local-billing-inbox.md) and [populated checkout restore](local-checkout-snapshot-recovery.md).
