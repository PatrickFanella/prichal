# Pausing new first-account requests

Schema 25 adds durable admission for new first-account invitation/recovery requests. This policy is separate from deployment admission, existing service/account holds and management sign-in. The explicit local operator command can close/reopen it without host access:

```sh
./scripts/dev.sh enrollment-admin status
./scripts/dev.sh enrollment-admin set --state closed --reason maintenance
./scripts/dev.sh enrollment-admin set --state open --reason maintenance
```

The command requires explicit local mode and a numeric loopback `pds_local` database. Supported reasons are `maintenance`, `safety_review`, `capacity_review` and `local_qualification`. Effective changes and audits commit atomically; repeated state changes add no audit. Missing policy, recovery holds and failed audits refuse new admission/change. No owner, customer CLI or HTTP operator mutation can change the policy. The initial open value preserves the disposable local workflow; production enrollment remains independently unconfigured.

New request transactions hold a shared policy lock through intent/event/River commit. Closing waits for already-admitted transactions to commit or roll back. Matching retained retries are resolved before admission, while a changed recovery flag conflicts and a foreign owner is denied. Closed admission creates no new enrollment work, including replacement/recovery invitations. Reads and existing status refresh remain available. Already-committed jobs may finish, and already-issued native invitations remain valid until expiry. Closing does not revoke invitation codes or stop native account creation against an existing valid invitation.

The owner-only `GET /api/deployments/{id}/enrollment/admission` returns open/time after current ownership verification. Browser controls show a pause, preserve retained retries/status, and disable new preparation/recovery when closed or availability is unknown. Initial loading has separate guidance. Polls are bounded and do not overlap; superseded reads are ignored. Account passwords stay directly on the native PDS.

## Local qualification

Full smoke passed Node checks, frontend build and Go vet/race/PostgreSQL tests. Tests cover retained retry, new/recovery refusal, changed-payload/foreign-owner denial, already-admitted worker completion, reopen, shared/exclusive locking, concurrent one-audit changes, audit rollback, missing policy, recovery hold, owner-only reads and the unchanged production gate. The local command rejects remote/incorrect databases, production mode and incomplete change flags. Optional golangci-lint remains unavailable.

Actual drill `f2ce6efd-bee9-4e5b-8483-91f5a8affd75` completed at 2026-10-01T18:13:48Z. Closed admission rejected new invitation/recovery requests, matching retry returned its retained operation, changed/foreign requests were denied, and status/history/export reads stayed available. Separate reopen restored open admission. Two effective audits were added; enrollment operations/events, jobs, lifecycle work, deployment policy and both native repositories/media, containers and protected custody were unchanged.

A separate actual browser pause at 18:14:38Z showed the pause while retaining the verified account and enabled status control. Five viewport sizes had no document/account overflow: 320×568, 375×812, 812×375, 768×1024 and 1440×900. Reopen added two further effective audits, leaving four total. The browser check preserved four enrollment operations, 14 events, 79 jobs and open deployment admission. Metadata remains in `.local/enrollment-admission-browser.json`.

Temporary read-only browser responses exercised failed-account recovery with closed and unavailable policy. Both disabled new recovery; zero mutation requests were sent. Original fetch was restored and actual verified account/availability reloaded. An initial preview evaluation failed; a refreshed observed-route repeat passed at 18:20:16Z. These responses qualify UI behavior only, not a native failed-account recovery. Assistive screen-reader and broader failure journeys remain open.

Fresh recovery `693e74a0-5b6a-4518-9fb2-d71507cd82cf` matched all 47 schema-25 tables and sequences, including the admission policy/four audits, four enrollment operations/14 events and 79 jobs. Copied authentication was revoked and held worker startup refused. Reconciliation/dedicated-queue replay preserved copied default work, source management rows/sequences and native custody. Source writers were paused across capture/replay. Recovery copies remain retained.

Source API, worker and monitor resumed on schema 25 with both admission policies open and collaboration/exit/billing/operator fixture permissions disabled. Public signup limits, issued-invitation revocation, independent infrastructure, notifications and signed-off release qualification remain open. No native host operation or container/volume/recovery-copy deletion occurred.
