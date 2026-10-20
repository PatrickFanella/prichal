# Disposable native member deactivation

Issue [#117](https://git.subcult.tv/subculture-collective/prichal/issues/117) requires a member to keep working during a migration window and then be deactivated rather than deleted. This delivery qualifies the native operation needed at the end of that window. The restricted host broker described below now enforces the fixed local migration window. Management requests and scheduled reconciliation now connect to that broker. Browser removal controls remain unfinished.

## Fixed native operation

`backend/internal/nodeagent/member-exit-helper.cjs` supports only `native_status DID` and `deactivate DID`. It runs inside a disposable identity-enabled PDS and requires the literal local PLC fixture URL. The native administrator password remains inside that runtime. The helper reads SQLite in a read-only transaction, refuses the earliest owner account, unknown DIDs and accounts without recorded invitation use, and verifies the returned subject identity.

For an active member it calls only `com.atproto.admin.updateSubjectStatus` with a repository subject and `deactivated.applied=true`. The pinned PDS implementation calls its account manager without a deletion deadline. The helper refuses an existing takedown or unexpected active-account deletion policy, then independently confirms native status and the retained actor row. It never activates an account, clears a takedown or schedules deletion. An already-deactivated member is returned without mutation, preserving a user-selected deletion schedule and original deactivation timestamp. Failed or uncertain writes return no success result.

The read action is named `native_status` because Node treats `inspect` as its debugger command even when supplied after `-e`. Fixed callers must use the allowlisted action.

The restricted host broker resolves the DID from its retained accepted-invitation journal and enforces the migration deadline before invoking this primitive. The helper itself does not know the membership policy or elapsed migration window. It has no arbitrary-DID HTTP endpoint; the agent accepts only a retained ticket. The management worker supplies only that retained ticket.

## Qualification command

```sh
./scripts/pds-local.sh qualify-member-exit
```

This command allocates a new labeled internal Docker network, three resource-bounded containers and independent data/blob/protocol volumes. It publishes no ports, uses the pinned official PDS and Caddy images, and creates three synthetic accounts against the local PLC/mail fixture. It does not open the retained local fixture or a management database. Credentials and reports stay under ignored `.local/pds-member-exit/UUID/` with private permissions. Its finalizer stops only containers with the exact qualification label; it retains volumes and reports, including failures.

On Kvant, October 19, fixture `56365f97-9914-4b91-a952-7ec38e60a35b` passed:

- Owner and unknown-DID requests were refused before mutation.
- A member created a native post with a PNG blob before deactivation. The actor row stayed present with no deletion deadline; the owner row was unchanged.
- Logical actor-store SQLite content, signing-key files and blob bytes matched before/after deactivation and restart. The seven-file inventory hash was `1d9360163280d27dd397e55a24783a67fa1ddec9404308bf2bc8a08c4dbf9024`. SQLite tables are read transactionally so checkpointing does not create false file-byte differences; this comparison is not a backup/restore qualification.
- A write with the member's existing session returned HTTP 401 identifying deactivation. Replaying deactivation retained the timestamp.
- A separate member's existing user deactivation and future deletion schedule were unchanged.
- Native restart retained container identities and deactivation state, and owner login still succeeded.
- Generated credentials were absent from fixture logs. All three containers stopped, with no stop failures; their volumes remain retained.

The private receipt is `.local/pds-member-exit/56365f97-9914-4b91-a952-7ec38e60a35b/report.json`. Earlier failed fixture reports/volumes remain retained. Signup harness assumptions were corrected to short unique handles. The first read-action trial hit Node's reserved debugger command before member deactivation was attempted; the final action name is qualified above.

Three Node boundary tests also cover owner/foreign/uninvited/nonlocal refusal, arbitrary actions, preserved existing deactivation, policy conflicts, foreign status, failed writes and lost responses. Full pinned smoke passed with 50 Node tests, frontend build, Go vet and all Go race packages; the final renamed read action passed its focused tests and the native fixture. Shell and JavaScript syntax checks passed.

## Restricted host migration window

`POST /v1/instances/{instanceId}/members/exit` accepts only `{ "ticket": "accepted-invitation-uuid" }` through the existing worker mTLS boundary. The registered-node client verifies the certificate identity and key pin. The Docker implementation requires isolated local identity mode and a healthy, unheld runtime, refuses public pilot mode, and resolves the member DID from the retained accepted ticket. Unknown or pending tickets fail before native execution. Requests cannot supply a DID, deadline or immediate-deactivation flag.

The first call uses `native_status` to confirm the member and atomically persists a private, versioned journal with a fixed 30-day deadline. Repeated calls and agent reconstruction retain the original deadline; calls before it do not mutate the native account. A call after the deadline invokes the fixed native helper and records completion only after confirmed native deactivation and journal persistence. Uncertain native results or receipt-write failures return no success; replay can reconcile the helper's idempotent result. The management worker below retains scheduled work to call again when due. The host broker itself has no autonomous timer.

The journal is bounded to 25 members, rejects duplicate identities/tickets, changed bindings, shortened windows, unknown fields, corrupt JSON, symlinks and public permissions. Its entries remain retained after deactivation. This does not free quota or delete data. The local 30-day constant qualifies the proposed window; production policy and organization termination remain separate decisions. Deadline enforcement trusts the host clock; callers cannot supply a clock value.

Go race tests cover the fixed Docker helper/ticket binding, pre-deadline refusal, restart and clock-backward replay, lost native responses, receipt-write failure after native success, corrupt/unsafe journal refusal, strict HTTP bodies and authenticated client refusal of foreign peers or premature completion. Elapsed time is simulated in tests through an unexported clock argument. These tests do not claim a physical 30-day soak or a combined native broker/management/browser run; the separate native helper fixture above proves the underlying PDS operation.

## Local management requests and reconciliation

Schema 38 adds independent `member_removals` and `member_removal_events` tables. Accepted invitation rows, public identity and quota count are preserved. The migration creates no removal requests for existing accounts and does not infer historical actors.

| Route | Purpose |
|---|---|
| `POST /api/member-invitations/{id}/removal` | Record removal for this accepted invitation with `{ "idempotencyKey": "UUID" }`. |
| `GET /api/member-invitations/{id}/removal` | Read the retained request, original request key, host window, completion and newest 100 audit events. Returns 404 if no request exists or access is denied. |

These routes use the existing local-only member session, exact origin and CSRF boundary. Current owners can request removal; active organization administrators require the existing independent `PDS_LOCAL_MEMBER_ADMINISTRATION` opt-in alongside organization access. Viewers, revoked or foreign members are denied. An organization lock serializes permission checks with role revocation. Previously authorized work remains valid after the initiating manager loses access; current reads and retries require permission again. No member password, email, posting or signing access is added.

A new request requires an accepted retained identity and ready managed deployment, and refuses a recovery hold or existing hosting exit. The accepted invitation ticket, canonical owner, initiating actor, idempotency key, audit event and River job are committed atomically. Repeating the original key returns the same request without another job or event. A different key for the same member, or reuse of a key for another account on the same deployment, conflicts. Existing native account restrictions remain authoritative; removal never activates a deactivated account.

The local worker authenticates the registered node and resolves only this retained ticket. It verifies the returned DID, exact 30-day interval and original host timestamps. Once a window is recorded, the job snoozes until its deadline without contacting the native account. It records deactivation only after the broker confirms it. Ready deployment, canonical ownership, retained accepted identity, hosting exit and recovery checks run again before dispatch. Organization hosting termination is not implemented by this route; overlapping hosting exit pauses member work for review.

Transport, identity or checkpoint failure returns a redacted error. After five failed attempts, the retained request enters `review` without claiming native completion or erasing its known deadline. Posting the original request key retries reconciliation with the same invitation ticket. A confirmed host window survives retries, and an uncertain host result can be recovered from the host journal. Worker audit events have no manager actor; request and explicit retry events identify the actual current manager. No cancellation, shortened window or account deletion is offered. Deactivated members remain in the accepted roster and continue to count toward its ceiling.

PostgreSQL race tests cover concurrent idempotency, request/audit/job atomicity including failed queue insertion, manager scope and revocation, real actor versus canonical owner, strict HTTP/session/CSRF/local boundaries, premature or foreign completion, changed host deadlines, worker reconstruction/replay, failed checkpoint recovery, review/retry, recovery hold and ownership/runtime/identity drift. The schema-37 upgrade retains accepted accounts and creates no removal intent; historical migration fixtures now remove only empty dependent tables in disposable allocator-owned databases. Tests simulate elapsed time and native broker results. A real River worker also persisted its future reconciliation schedule at the host deadline before stopping. These checks do not prove a physical 30-day timer run or combined real native management flow.

Upgrade through normal startup migration. A schema-37 binary rejects schema 38; rollback requires a compatible binary or separately qualified restore. Do not decrement the schema marker or drop populated removal/audit tables. No running API, worker, production database or native account was upgraded for this source delivery.

## Remaining #117 work

Browser review and combined native/management/browser qualification remain open. The complete exit policy must cover organization hosting termination and handles under verified organization domains. Administrator/native end-to-end qualification, plan limits and production enablement remain separate. No issue is closed by this native primitive.
