# Owner-scoped invitation withdrawal intent

The local browser API accepts `POST /api/enrollments/{id}/withdraw` with an empty JSON object. It requires the original enrollment owner, current organization owner, a ready managed deployment, browser Origin and CSRF. Deployment-scoped CLI credentials cannot use it. Production enrollment remains unavailable. The [restricted agent command](local-enrollment-withdrawal-agent.md) performs host work; the API has no Docker access or host credentials.

The retained enrollment UUID identifies withdrawal intent. Issued, expired and failed requests can request withdrawal. A request still preparing its first invitation returns a conflict because the agent requires an exact retained ticket. An existing `account_created` or `verified` result returns status without adding intent, jobs or audits. Preparing-request cancellation is a separate remaining task.

## Transaction and confirmation

Schema 28 adds a withdrawal timestamp, an attempt count bounded to three and the `withdrawing`/`withdrawn` states. Existing rows retain their prior fields with no withdrawal requested and zero attempts. Database checks prevent withdrawal states without intent and prevent an invitation ciphertext from remaining after withdrawal intent.

The transaction locks the deployment before enrollment, matching the worker and refresh path. It clears the stored invitation code and commits `withdrawing`, one audit event and one River job together. Repeating a pending or confirmed withdrawal adds no job or audit. Queue or audit failure rolls back the entire request. New enrollment admission may be closed while withdrawal remains available.

The worker reads current intent under the same locks. Already-queued issuance/refresh work dispatches withdrawal once intent exists. A confirmed withdrawn/superseded result does not repeat host work. Native confirmation may report an account that completed during the drain; the worker retains that account. An unconfirmed withdrawal remains failed with its intent retained and no invitation code. New-ticket recovery is blocked until withdrawal is confirmed. Each explicitly accepted withdrawal/retry has at most three worker attempts, and the operation accepts at most three such requests.

Failure recording checks the accepted withdrawal attempt number. An older issuance or withdrawal failure cannot fail a later owner request accepted after that worker released its locks. A failed withdrawal does not silently issue again. Confirmed withdrawal permits separately authorized, bounded recovery with a new ticket.

Enrollment reads now hold shared deployment/organization and enrollment locks through code decoding and event retrieval. They serialize with accepted withdrawal and ownership changes. Missing/held recovery state denies invitation disclosure, intent, refresh, worker execution and failure checkpoints. Historical guidance distinguishes withdrawal requested from confirmed withdrawal. Responses include `withdrawalRequested` and `withdrawalAttempts` for later browser retry guidance.

## Verification and retained state

The full local smoke gate and enrollment race tests passed. Disposable integration databases qualify concurrent idempotency, queue/audit rollback, old-failure protection, retry bounds, replacement/refresh refusal, original/current ownership, recovery holds, missing recovery ledger, browser authorization/CSRF, CLI refusal and production-route refusal. Schema-27 upgrade preserves existing intent, and database constraints reject unrecorded withdrawal. Injected native results qualify confirmation and completed-account preservation; they do not qualify a fresh empty PDS.

Focused coverage is 81.8% for withdrawal intent, 86.7% for deployment/scope locking, 100% for transactional recovery checks and 85.7% for failure recording. Broader enrollment coverage is 76.4%. The later audit-failure regression passed the focused race gate after the full smoke run. Optional `golangci-lint` remains unavailable.

Actual API preservation drill `14a3b105-cc5a-49d1-b834-2ec6e74f8a1a` completed at 2026-10-01T21:30:58Z. Alice/Bob repeated withdrawal requests for their already verified accounts returned status without adding intent, attempts, jobs or audits. Foreign scope, invalid CSRF and unsupported authority were denied. Source enrollment/deployment work and native signed data/media, container start times and custody matched the baseline. Authentication activity changed intentionally during fixture sign-in.

Before schema 28, local API/worker/monitor stopped and the retained schema-27 recovery copy remained available. After migration, fresh recovery `9ebb9edf-a298-4599-bdb9-37777a3dcf92` matched all 51 management/River inventory tables and sequences. Copied authentication was revoked and held worker startup refused. Reconciliation/dedicated replay preserved source rows/sequences, default jobs and native signed data/media/custody. Management writers stayed paused through capture/replay, then resumed on schema 28 with fixture permissions disabled and admission policies open. The original capture report records the held snapshot; the separate replay evidence and recovery ledger record reconciliation.

Optional API preservation qualification:

```sh
node scripts/qualify-enrollment-withdrawal-intent.mjs
```

The script accepts only the two fixed disposable local instances and requires already verified accounts. Private report metadata stays under ignored `.local/enrollment-withdrawal-intent/`. It cannot qualify cancellation or withdrawal of a fresh invitation.

## Remaining gates and rollback

[Browser review/retry](local-enrollment-withdrawal-workspace.md) has fixture and actual unresolved-ticket reload checks. Actual fresh native empty-PDS/browser withdrawal, cancelled preparation, drain/restart failure and signup/withdrawal races remain open. Three retained native slots remain occupied. No tenant, volume, recovery copy or signing custody was deleted to make room. Independent storage, live provider/domain decisions, production agent/operator policy and public release remain deferred. Issue #26 remains open.

Migration 28 is forward-only. Older management binaries refuse the newer schema. Retain the pre-migration encrypted recovery copy for a reviewed restoration; do not drop the new columns or edit source operation history as a rollback. Agents predating the restricted withdrawal command cannot confirm these jobs, so install the designated agent before exposing the control.
