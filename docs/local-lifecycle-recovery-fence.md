# Local lifecycle recovery fences

Status: locally implemented and verified on October 1, 2026. Independent recovery and production operator qualification remain open.

Lifecycle submission now has an eight-second deadline and checks recovery after its deployment lock, before exact receipt replay and before committing a new intent/audit/queued job. Held or missing recovery state cannot become new lifecycle authority after a database wait. The worker checks current recovery after deployment/operation locks and immediately before service or account RPC dispatch. Uncertain recovery snoozes the original job without adding a new intent or marking it failed. Terminal failure recording checks recovery inside its transaction and before commit, rolling back a label or audit if recovery became held during the wait.

This fences work still waiting for dispatch. It does not undo an RPC that already began with authority; the retained generation and ticket remain the recovery identity. Existing confirmed native checkpoints retain their behavior. Management writers must still be quiesced before stable snapshot capture.

## Verification

A new request, an exact retry and a worker dispatch first reproduced the lock-wait race: introducing a hold while each waited for the deployment lock still allowed intent or host execution. Six fixed tests cover held and missing-ledger variants, preserving complete operation/event/job rows and performing zero native fixture calls. Additional audit-lock tests introduce a hold while recording a new intent or a terminal failure: intent, audit and queue inserts roll back; the terminal failure snoozes and retains its original pending operation after the previously authorized failed fixture RPC. Focused lifecycle race checks and the full local smoke gate passed.

Only allocator-owned databases and mock agent calls were used for the positive race checks. No source recovery hold, native lifecycle action, resource deletion or fixture-permission change was performed. Private evidence remains ignored under `.local/`. Live recovery custody, notifications and production operator identity remain unqualified.

The ordinary local API and restricted-agent worker restarted with the updated code. API readiness returned schema 30 and source work/native/custody and saved-choice inventories matched. Ordinary fixture permissions remain unchanged.
