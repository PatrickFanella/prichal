# Queued enrollment cancellation

Status: locally implemented and verified on October 1, 2026. Browser review has local keyboard/DOM evidence. Production qualification remains open.

An owner can POST an empty JSON object to `/api/enrollments/{id}/cancel` to cancel a pending first-account request before native dispatch. The original operation UUID identifies the request. Authentication requires the browser owner session, allowed origin and CSRF token; deployment-scoped CLI credentials have no authority. The route stays unavailable outside local mode.

Cancellation locks the ready agent deployment and current organization before the enrollment row. Both the original and current owner must match. It changes the request and writes one audit event atomically. Repeating an accepted cancellation returns the retained result without new events or jobs. The original queue job remains and performs no native action. Refresh and invitation withdrawal refuse cancelled requests. A separately authorized recovery request uses a new ticket and the existing three-per-hour bound.

Before any native enrollment RPC, the worker commits `dispatch_started_at` while holding the same deployment/enrollment locks. Cancellation can win before that commit. After the checkpoint, cancellation returns conflict even if the native response is lost and management still says pending. The worker rechecks owner, runtime and recovery authority after reservation. An unavailable checkpoint prevents host execution. Failure recording cannot overwrite cancellation.

Schema 29 adds that checkpoint and the cancelled state. Every retained row receives its previous `updated_at` as a conservative dispatch marker. Previous pending work might already have reached native execution, so upgrade does not make it cancellable. Existing row fields remain unchanged. New requests start without a marker. Database constraints reject cancellation with dispatch, invitation, withdrawal or account metadata. Status exposes `canCancel`; the browser contract rejects inconsistent eligibility and supports retained cancelled status and bounded recovery.

## Verification

The first regression failed because schema 28 had no durable dispatch marker after a native failure. Focused enrollment/migration race checks and the complete `./scripts/dev.sh smoke` passed after implementation. Integration tests cover concurrent cancellation, original/current ownership, stable retries, retained-job refusal, supersession, audit rollback, held/missing recovery ledgers, native checkpoint failure and an in-flight/lost response. HTTP checks cover session, origin, CSRF, foreign scope, malformed bodies, admission pause, production refusal and dispatched conflict. Contract tests cover queued eligibility and cancelled metadata. The previous checkpoint-loss fixture now rejects only the native-result update, so it still proves replay after actual fixture RPC execution.

Management writers were paused for the local upgrade. All four retained enrollment rows preserved their previous fields and received conservative dispatch markers. Fresh recovery copy `7fbe2fdf-3c88-4f89-845f-db065dca1b74` matched 51 tables and sequences, revoked copied authentication and refused held worker startup. Reconciliation/replay preserved source management tables/sequences, default jobs and native signed data/media/custody. Its original capture report remains held; the separate replay report records readiness.

API, worker and monitor resumed on schema 29 with normal fixture permissions off and admission open. Actual preservation run `2976f83d-6210-40be-9306-d6dde219612a` at 22:09:07Z refused four repeated cancellation requests against the two verified accounts, enforced scope/CSRF/body boundaries and preserved management work, native containers/start times, signed records/media and custody files. These are refusal checks, not positive cancellation of a source tenant. Private reports are in ignored `.local/`.

The actual T3 workspace showed verified account status without an enrollment error, injected fixture, retained withdrawal key or document overflow at 1440×900. Screenshot and screen-reader qualification remain open. No retained tenant, native slot, export or recovery copy was deleted or reset.

## Remaining work

The [browser review](local-enrollment-cancellation-workspace.md) now has keyboard/error/reload and exact-ticket retry evidence. Visual and assistive checks remain open. Positive cancellation is currently exercised in disposable database/agent fixtures. An actual designated empty PDS and independent live environment remain necessary for native withdrawal and broader release gates. This feature does not delete an account, close public signup or prove public DNS, TLS, email or federation.
