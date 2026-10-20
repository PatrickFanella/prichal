# Disposable native member deactivation

Issue [#117](https://git.subcult.tv/subculture-collective/prichal/issues/117) requires a member to keep working during a migration window and then be deactivated rather than deleted. This delivery qualifies the native operation needed at the end of that window. The restricted host broker described below now enforces the fixed local migration window. Management requests and scheduled reconciliation now connect to that broker. Browser removal review and retained-key retries are described below.

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
| `GET /api/member-invitations/{id}/removal` | Read the retained request, original request key, host window, completion and newest 100 audit events. Returns an explicit target-bound `not_requested` status for an authorized accepted member with no removal, and 404 for denied access. |

These routes use the existing local-only member session, exact origin and CSRF boundary. Current owners can request removal; active organization administrators require the existing independent `PDS_LOCAL_MEMBER_ADMINISTRATION` opt-in alongside organization access. Viewers, revoked or foreign members are denied. An organization lock serializes permission checks with role revocation. Previously authorized work remains valid after the initiating manager loses access; current reads and retries require permission again. No member password, email, posting or signing access is added.

A new request requires an accepted retained identity and ready managed deployment, and refuses a recovery hold or existing hosting exit. The accepted invitation ticket, canonical owner, initiating actor, idempotency key, audit event and River job are committed atomically. Repeating the original key returns the same request without another job or event. A different key for the same member, or reuse of a key for another account on the same deployment, conflicts. Existing native account restrictions remain authoritative; removal never activates a deactivated account.

The local worker authenticates the registered node and resolves only this retained ticket. It verifies the returned DID, exact 30-day interval and original host timestamps. Once a window is recorded, the job snoozes until its deadline without contacting the native account. It records deactivation only after the broker confirms it. Ready deployment, canonical ownership, retained accepted identity, hosting exit and recovery checks run again before dispatch. Organization hosting termination is not implemented by this route; overlapping hosting exit pauses member work for review.

Transport, identity or checkpoint failure returns a redacted error. After five failed attempts, the retained request enters `review` without claiming native completion or erasing its known deadline. Posting the original request key retries reconciliation with the same invitation ticket. A confirmed host window survives retries, and an uncertain host result can be recovered from the host journal. Worker audit events have no manager actor; request and explicit retry events identify the actual current manager. No cancellation, shortened window or account deletion is offered. Deactivated members remain in the accepted roster and continue to count toward its ceiling.

PostgreSQL race tests cover concurrent idempotency, request/audit/job atomicity including failed queue insertion, manager scope and revocation, real actor versus canonical owner, strict HTTP/session/CSRF/local boundaries, premature or foreign completion, changed host deadlines, worker reconstruction/replay, failed checkpoint recovery, review/retry, recovery hold and ownership/runtime/identity drift. The schema-37 upgrade retains accepted accounts and creates no removal intent; historical migration fixtures now remove only empty dependent tables in disposable allocator-owned databases. Tests simulate elapsed time and native broker results. A real River worker also persisted its future reconciliation schedule at the host deadline before stopping. These checks do not prove a physical 30-day timer run or combined real native management flow.

Upgrade through normal startup migration. A schema-37 binary rejects schema 38; rollback requires a compatible binary or separately qualified restore. Do not decrement the schema marker or drop populated removal/audit tables. No running API, worker, production database or native account was upgraded for this source delivery.

## Browser removal review — October 20

The accepted-member roster opens one removal panel at a time, independently of invitation history. Closed panels fetch no removal status. Owners and explicitly enabled current administrators use the same server authorization as invitations. The panel shows the retained request, confirmed migration deadline and newest 100 audit events. Only explicit absence or a review outcome offers a mutation; requested, migration-window and deactivated receipts are read-only.

The GET route now returns `not_requested` with the exact deployment, accepted invitation and DID only after current accepted-member authorization. Denied, unknown, pending or inconsistent identities still return an error. This separates absence from access denial and unsupported older APIs. The browser rejects missing/foreign identifiers, malformed actors, shortened windows and premature completion. The matching API must be deployed with this UI; an older 404-only route pauses removal controls.

Review identifies the member and explains the fixed window, existing account restrictions, deactivation with retained data/quota, unchanged handle and lack of cancellation. A checkbox is required before confirmation. A fresh read precedes every POST, which carries only an idempotency key and CSRF token. Only request identifiers are kept in session storage. An uncertain response retains its key across panel/session remounts; a review retry uses the original server key. Blocked storage prevents mutation. A changed management session or loss of the parent roster unmounts the panel and aborts its request; reopening reconciles its retained key. Polling is read-only, bounded to one active read per open panel, and stops on unmount. Failed, denied or malformed reads clear the receipt and controls.

Controlled T3 browser checks used the actual React components with synthetic fetch responses. They passed review acknowledgement, keyboard Tab/Space, focused headings/errors and focus return; lost POST reply and same-key retry; remount without another POST; abort-on-unmount with retained-key reconciliation; blocked-storage refusal; denied/foreign data cleanup; read-only completed receipts; closed-panel idleness and parent-denial unmount. Integrated roster/review layouts fit 320, 768 and 1280 CSS pixels without horizontal overflow. Private evidence is `.local/member-removal-browser.json`. These checks did not mutate a native account or management service and do not establish combined native qualification or a physical 30-day run.

## Real initial migration window — October 20

`./scripts/dev.sh qualify-member-removal` runs the opt-in Go race qualification against the separate disposable local PostgreSQL started by `./scripts/dev.sh db`. The pinned Go/Node toolchains and local Docker Unix socket are required. It creates a fresh allocator-owned management database, tenant UUID and private `.local/member-removal-native/UUID/` directory. Ordinary smoke and CI skip the native test unless its explicit qualification flag is set.

The real agent policy requires the existing canonical local PLC/mail fixture, with its pinned image, Compose identity and fixed command. The harness preserves that fixture's existing records and configuration, starts it only if stopped, and restores its original running/stopped state. It admits exactly one additional tenant within the measured host limit and the existing bounded local ceiling; retained stopped tenants still count, and none are pruned or started. The fresh tenant has an internal data network, restricted native/guard/ingress resources and only its normal loopback HTTPS endpoint. It shares the policy-pinned PLC/mail fixture; it is not a separate public identity service.

The management owners, session and organization administrator are synthetic test fixtures. The owner account is created on the real pinned PDS through the guarded first-account broker and then reconciled to close that lease. Its observed identity seeds the management first-account precondition. Member invitation and removal go through the actual session/origin/CSRF-protected management HTTP handlers in process. Workers invoke their normal `Work` methods against real PostgreSQL and the real registered mTLS node client/handler. This is not an OAuth sign-in, separately deployed API/worker process, browser-native run or live River scheduling test.

Fixture `028d3448-f1b6-4573-a5a4-556ac7fec3e9` passed under Go's race detector. The opted-in administrator requested an invitation, the member created its own native account, and management retained the accepted DID. The member created a native post before removal, another during the confirmed migration window and another after reconstructing the authenticated agent server and Docker broker from retained disk state. Removal retained its original key, ticket, DID and exact 30-day deadline, including same-key management replay. The audit identified the initiating administrator and a separate null-actor worker observation; accepted quota remained one.

The deadline was `2026-11-19T05:37:12.012Z`. No clock or journal was backdated, and no native deactivation was attempted. This proves the initial management-to-native window and retained-state reconstruction; a real 30-day elapsed run and combined post-deadline management deactivation remain unobserved. The separate native helper and simulated-deadline tests retain their narrower evidence above.

The private receipt is `.local/member-removal-native/20261020-fourth/report.json`. All three fresh tenant containers stopped, volumes/state were retained, and the temporary tenant attachment was removed from the shared protocol fixture. Independent readback confirmed the protocol fixture returned to stopped with its original four network attachments. The allocator-owned management database was dropped by the existing test harness. Three failed harness attempts remain retained and stopped: setup was corrected to wait for native health, use the exact management session-token length and reconcile the owner's completed lease before member signup. Their temporary protocol attachments were independently removed. No existing tenant, public service, production database or release was changed.

## Read-only retained exit inspection

The registered worker client can call `MemberExitStatus` through `POST /v1/instances/{id}/members/exit/status` with only the accepted invitation ticket. The route uses the same pinned node identity and worker mTLS boundary, strict bounded JSON body and no-query allowlist as removal. It selects a separate inspection method; an older agent without that method returns unavailable rather than falling back to removal.

The restricted local broker requires healthy, unheld hosting, an accepted member journal and an existing private exit journal. It resolves the DID from retained state and runs only the fixed helper's `native_status` action. Inspection preserves the original state and timestamps even when a window is overdue or native deactivation happened before an uncertain journal checkpoint. A completed journal requires native deactivation confirmation. Missing or unsafe journals, changed identities and unavailable native status return no successful observation. Member handle changes do not change the retained DID binding. No invitation codes, credentials or private native settings are returned.

Management recovery requires this read-only observation for every retained member removal before releasing its hold. Confirmed `migration_window` and `deactivated` rows must match the accepted member, current organization owner, verified host placement, original timestamps and host state exactly. Requested/review outcomes, missing observations and identity or deadline differences remain held for operator reconciliation. Reconciliation does not rewrite removal intent, audit, scheduling or host state, including overdue windows. Databases without removal rows retain their existing recovery path.

The node inspection tests use the existing emulated Docker transport and real mTLS HTTP boundary. Recovery tests use allocator-owned disposable PostgreSQL and a read-only observer fixture. These checks do not qualify a populated encrypted management/PDS restore, discover host exit journals absent from management's snapshot, prove a native elapsed window or enable production recovery. An uncertain request remains held; this comparison adds no operator override or automatic repair.

## Retained native host-metadata encryption and restoration — October 20

At source commit `2b28279a9c1c360158073a6d7eb1caffce8fbd84`, the three real host sidecars from native fixture `028d3448-f1b6-4573-a5a4-556ac7fec3e9` were copied through `memberRecoveryFiles`, archived as the exact fixed filename inventory, sealed with `sealRecovery`, authenticated with `openRecovery` and restored through `restoreMemberRecoveryFiles` into a fresh private candidate. All three restored SHA-256 values matched their source bytes. The retained exit remained `migration_window` with its original deadline, `2026-11-19T05:37:12.012Z`.

The authenticated bundle was 10,396 bytes with SHA-256 `718d8fd37de41f189e74adb06817ea769f581cf5c023c37599f01357586c097b`. A modified authentication tag was refused without publishing plaintext at the requested destination. The separate guard candidate directory contained only `members.json`; neither invitation nor exit journal was placed there. This was a file-layout check, not a running guard or restored-agent activation.

The private receipt is `.local/member-metadata-recovery/56303ff0-5131-4088-9dcc-e36c914d351c/report.json`; ciphertext, key and private checkpoint/candidate files remain beside it under the ignored private directory. Source hashes and the original stopped-container timestamps matched before and after the drill. No source journal, runtime, native account or management database was changed.

This qualifies encrypted restoration of the retained native host metadata only. The bundle and its key are on the same machine, so independent custody and retention are unqualified. PDS SQLite databases, signed repositories, blobs, native writes, management PostgreSQL, physical elapsed-window behavior and production recovery were outside this drill. Those wider gates remain open.

## Remaining #117 work

The initial real management/native window is qualified above. Combined browser/native and post-deadline management qualification remain open. The complete exit policy must cover organization hosting termination and handles under verified organization domains. Administrator/native end-to-end qualification, plan limits and production enablement remain separate. No issue is closed by this native primitive.
