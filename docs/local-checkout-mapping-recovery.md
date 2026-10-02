# Populated completion mapping snapshot recovery

Status: locally qualified on October 1, 2026 using allocator-owned disposable PostgreSQL and a synthetic SDK authority. No actual Stripe resource or paid access was created.

The explicit snapshot opt-in now captures a confirmed original checkout, one verified binding/worker audit/current unpaid observation, one applied completion notification, one pending delayed-payment notification, three original jobs and a fixture authentication session. A bounded in-memory custom pg_dump restores into an allocator-owned recovery database. Complete binding/observation/notification/request/audit/job ledgers match exactly before the hold, including original custody, first notification and explicit payment evidence.

Freshly constructed inbox/store/SDK adapters all use the copied database and original fixture configuration. Holding the copy revokes authentication. Applied and pending completion workers snooze, and duplicate delivery receives no acknowledgement. Both source and copied ledgers remain unchanged with zero provider calls. Host-free reconciliation makes no native agent call.

Applied receipt replay after reconciliation makes no provider call or ledger change. The real River worker then completes all three copied jobs: terminal checkout and applied completion need no RPC; pending completion makes two exact current graph reads. The original binding and original-owner worker audit remain singular and unchanged. The current copied observation records unpaid/past-due state with the pending event's provenance; neither event type nor status grants entitlement. Terminal receipt replay makes no further call. Original fixture ledgers remain unchanged and private payment/metadata content is absent.

Focused races and full smoke with `PDS_TEST_LOCAL_CHECKOUT_SNAPSHOT=true` passed. Default smoke skips the Docker-specific snapshot while retaining mapping, transaction and SDK checks. Source schema-34 readiness remains green with billing disabled and source work/native/custody matching. The qualification removes only allocator-owned test databases and retains safe 0600 scalar reports under ignored `.local/checkout-mapping-recovery-*/report.txt`. No snapshot, credential or raw provider response is logged. Optional golangci-lint remains unavailable.

Private evidence: `.local/completion-mapping-recovery-focused.log`, `.local/completion-mapping-recovery-smoke.log` and `.local/completion-mapping-recovery-source-after.json`. See [atomic mapping](local-checkout-completion-mapping.md), [source recovery](progress/local-m1-alpha.md) and [actual registry replay](local-checkout-registry-recovery.md).

Actual synthetic authority completion/browser journey, positive-state responsive coverage, commercial plan/grace/entitlement policy, Stripe sandbox, independent backup custody and live qualification remain open. This fixture does not establish real-provider or off-host recovery durability.
