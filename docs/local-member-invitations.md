# Local member invitations

Issue [#117](https://git.subcult.tv/PatrickFanella/pds-ass/issues/117). An owner whose first account exists can invite members to create their own native accounts on the same PDS. Each invitation is one native single-use code. Members enter their password and email directly on the PDS; management never receives them. The owner browser controls below use these existing local routes. Member removal remains separate work. Administrator invitation permission now has its own local opt-in.

## Owner routes

| Route | Purpose |
|---|---|
| `POST /api/deployments/{id}/members/invitations` | Request an invitation with `{"idempotencyKey": UUID}`. A repeated key returns the original invitation. |
| `GET /api/deployments/{id}/members/admission` | Current enrollment request policy after invitation-management authorization. |
| `GET /api/deployments/{id}/members/invitations` | Ceiling, complete recorded accepted-member roster/count, open invitations and the newest 100 invitations. |
| `GET /api/member-invitations/{id}` | One invitation with its code while open, and its history. |
| `POST /api/member-invitations/{id}/refresh` | Ask the node whether the code was used. At most once every five seconds. |
| `POST /api/member-invitations/{id}/withdraw` | Disable an unused open code. A request still pending returns its current status; withdraw it once it is ready. |

Routes are local-only, with the same session, origin and CSRF checks as first-account enrollment. Owners have access by default; current administrators require the separate opt-in described below. A new request needs a ready agent deployment, a recorded first account, open enrollment admission, no recovery hold and no unreversed hosting exit. Each deployment allows ten new requests per hour. Pausing enrollment admission pauses member requests too; open invitations stay valid until they expire.

States are `pending`, `invitation_ready`, `accepted`, `expired`, `withdrawing`, `withdrawn`, `ceiling_reached`, `owner_required` and `failed`. Schema version 35 stores invitations and their events. Codes are encrypted with the enrollment key, scoped to the invitation, deployment and organization, and cleared once an invitation leaves `invitation_ready`. Accepted rows keep only the member's DID and handle; one DID can be accepted once per deployment.

## Native boundary

The worker calls two fixed agent operations over the registered node's mutual TLS: issue or report an invitation, and withdraw it. Requests carry only the instance and invitation UUIDs.

- **Ceiling.** The fixed ceiling is 25 members per PDS, a placeholder until plan limits (#19) exist. Management counts open, pending and accepted invitations. The node counts native accounts plus usable codes: it issues a code only while members plus usable codes stay below the ceiling. Before counting, it disables any usable code it is not tracking, such as one from a lost response. Each account needs one code use, so concurrent signups cannot exceed the ceiling. `PDS_AGENT_MEMBER_CEILING` lowers it for local qualification.
- **Withdrawal and expiry.** The PDS checks a code and records its use in one SQLite transaction, so a code disabled by the node cannot create an account afterwards. Withdrawal disables the code and then reads its use: a code used first becomes an accepted member and no account is changed. Codes last 72 hours. A scheduled job runs at expiry and the node disables the code. Each issue or refresh also disables any expired code first.
- **Admission guard.** The guard now reads a second private policy, `members.json`, next to the first-account lease. It admits a signup while at least one member invitation is current, the owner account exists and owner plus members are below the ceiling. The policy contains tickets and windows, never codes. The guard cannot tell which code a request carries. If the worker is unavailable, an expired code keeps working while another invitation is open, until the next issue or refresh disables it.
- **Owner account.** The owner's first account is the earliest native account. First-account status, moderation inspection and owner takedown use it, so they keep working when members exist. Takedown does not change member accounts. Customer export selects its account by DID instead of requiring a single account.

## Upgrading an existing local deployment

The guard's embedded policy changed, so an existing guard no longer matches. Stop the agent, retain the old guard and reconcile:

```sh
node scripts/prepare-local-enrollment-guard.mjs INSTANCE_UUID member-admission
./scripts/agent-local.sh run
```

The script saves the old container's inspection in `.local/local-enrollment-guard-upgrades/INSTANCE_UUID/member-admission/` and renames it with a `-before-member-admission` suffix. Reconciliation creates the new guard. No volume or credential changes.

## Local qualification

On Kvant (Linux), October 2:

- Gate: `./scripts/dev.sh smoke` passed: Node tests, web build, `go vet`, and race tests with PostgreSQL in all 24 packages that have tests. Two earlier full runs failed only because a disposable database drop exceeded the harness's five-second cleanup limit under parallel load; that limit is now 30 seconds. New Go tests cover owner scope, replay, admission, hourly limit, ceiling, hosting exit, encryption, expiry scheduling, acceptance, duplicate identity, withdrawal races, regression and failure messages, plus the agent broker, its routes and response validation.
- Node: 41 tests, including the guard policy, the member helper over a model of the native tables, and updated first-account and moderation helpers.
- Native: `node scripts/qualify-member-native.mjs INSTANCE_UUID` ran the fixed helpers inside a real pinned PDS (0.5.36). It confirmed: no code before the owner account, untracked code disabled, ceiling counting usable codes and members, single-use codes with reported use, a disabled code rejected at signup, and owner status, moderation inspection and export with a member present. Report: `.local/member-native/1fe9e0a5-3a1d-472f-b41b-a5905f5f61d3/report.json`. Account credentials stay in that private directory.

`node scripts/qualify-member-invitations.mjs DEPLOYMENT_UUID` drives the whole journey through the owner API, worker, agent, guard and native signup. The initial October 2 attempt was blocked by native Linux bind-mount permissions. Issue #125 fixed that reader identity contract, and the first-account and member journeys passed on October 8 with a two-member ceiling. [Native qualification and retained-reader upgrade](local-private-file-readers.md) record the reports, operator-assisted retry and limits. That existing native evidence was not rerun for the browser delivery below.

Still open after the owner browser delivery: member removal and the 30-day migration window, administrator permission (addressed by the separate opt-in below), organization-domain handles (#10), live expiry of a real 72-hour code, public DNS, TLS, email and federation.

## Owner browser controls — October 19

A member panel appears after management confirms the owner's first account. It lists the newest 100 invitations, accepted member handles/DIDs, the fixed member ceiling and open-code count. Owners can prepare a single-use invitation, request a native use check, and review withdrawal of a current unused code. It links to the validated PDS account page in a separate tab without an opener or referrer; invitation codes never enter the URL. Members enter their credentials directly on the PDS.

Withdrawal requires a focused review and acknowledgement that an account created first is preserved. It does not remove an accepted member. There are no member-password, email or posting controls. New requests follow the existing enrollment admission policy; server-side ownership, capacity, hourly limit, recovery and hosting-exit checks remain authoritative.

Only UUID request identifiers are retained in session storage for same-tab reload recovery. The browser writes the request key before submission, reuses it after an uncertain response, and refuses submission when it cannot retain the key. A retained uncertain withdrawal hides all codes in the panel until fresh status confirms its result or the owner reviews a retry of that exact invitation. If it falls outside the newest 100 rows, the browser reads its exact ID separately. No mutation runs automatically on reload or polling. Closing a tab can lose these request identifiers; after reopening, inspect existing invitations before preparing another.

Polling is read-only, bounded to one active load and cancelled on unmount. Failed, denied or malformed invitation reads clear the panel's codes and pause controls. Response validation checks deployment identity, member state/identity, duplicate IDs and the fixed managed localhost account endpoint. Codes disappear after local expiry even before the next server read. A changed management session remounts the panel, and a lost first-account authorization removes it.

Verification on Kvant used the actual React components in the T3 collaborative browser with synthetic responses, without native/API mutations. Checks passed for all nine invitation states; lost submission and withdrawal responses; stable request keys across retries and parent remounts; explicit refresh; review acknowledgement/focus and close-focus return; accepted identities; expiry; denied and malformed reads; admission pause; recovery; blocked storage; no stored codes or credential inputs; and first-account integration. Actual Tab/Space navigation reached the checkbox and enabled confirmation. At 320, 768 and 1280 CSS pixels the panel had no horizontal overflow; the nested first-account layout also fit at 320 pixels. Private result: `.local/member-browser-checks.json`.

The pinned `./scripts/dev.sh smoke` gate passed: 46 Node tests, frontend build, Go vet and all race-enabled Go packages. A final frontend build passed after the policy-denial refinement. The separate qualification PostgreSQL had zero generated smoke/recovery databases and was stopped with its volume retained. The incidental package-manager lockfile change was removed. Local checks do not establish PR-head CI, merge, deployment, live expiry, organization-domain handles, administrator permission or member-removal policy. Issue #117 remains open.


## Administrator invitation permission — October 19

Ordinary collaboration remains a metadata rehearsal. To let active organization administrators manage member invitations, the local operator must independently enable both flags on the API:

```sh
PDS_LOCAL_ORGANIZATION_ACCESS=true PDS_LOCAL_MEMBER_ADMINISTRATION=true ./scripts/dev.sh api
```

The new flag defaults off. Startup rejects a true value outside local mode, without collaboration enabled, or any value other than true/false. Turning it off removes administrator invitation access without changing membership or native accounts. Production member routes remain unavailable. Native posting, signing, passwords, billing, exports, first-account enrollment and hosting exit retain their existing permissions.

An enabled current administrator can list/read invitations and accepted identities, issue a code, request a use check, or withdraw an unused code. Viewers, revoked members, members of another organization and former owners without a current permitted role receive no invitation access. Every request checks current organization ownership or active administrator membership. The organization lock serializes admitted actions with ordinary role changes/revocation. Revocation prevents subsequent requests; it does not undo work durably authorized before the revocation.

Invitations retain the canonical organization owner in `owner_id`, so the existing worker ownership check and encrypted code scope remain intact. The actual actor is recorded separately for issue, explicit refresh and effective withdrawal. Worker observations have a null actor rather than impersonating a manager. Migration 37 adds nullable `member_invitation_events.actor_id`; existing events remain unknown and are not backfilled from current ownership. Apply through normal startup migration. An older binary rejects schema 37; rollback needs a compatible binary or a separately qualified management restore, not a schema-version decrement.

Owners and administrators can open a ready managed server's member panel from shared server status. Closed panels do not fetch invitation data. Viewer UI has no member entry, but server authorization remains authoritative. The member panel now reads `/members/admission` instead of the owner-only first-account policy route. It remains available to owners from their first-account screen. Losing shared status unmounts its panel, and losing invitation access clears its codes.

Verification includes race-enabled real PostgreSQL tests for independent opt-in, all six read/mutation/policy boundaries, viewer/revoked denial, canonical worker ownership, issue/refresh/withdrawal actor attribution, ordinary revocation serialization, legacy migration and HTTP session/CSRF/production gates. Controlled browser responses checked explicit opening, administrator submission with CSRF, disabled/revoked access, recovery, viewer absence and shared-read failure cleanup. Shared-panel layouts fit 320, 768 and 1280 CSS pixels. Private browser result: `.local/member-administrator-browser.json`. No native runtime, production flag or live membership was changed. Member removal and its migration window, plan limits, organization-domain handles and live administrator/native qualification remain open under #117.

The full pinned smoke gate passed after the refresh-audit expectation was updated: 46 Node tests, frontend build, Go vet and all Go race packages. A final focused member race run passed the additional foreign-organization refusal and not-found handling. Qualification uses a separate disposable PostgreSQL; no schema migration or flag change was applied to a live service.


## Accepted roster independent of invitation history — October 19

The newest-100 invitation window previously also limited which accepted member identities were visible. An accepted member could disappear from the panel after enough later invitations, while still contributing to its member count.

The existing summary now includes an `accounts` roster with invitation ID, public DID and handle for every recorded accepted member. It is fetched independently of the history window under the same current owner/administrator permission and organization/deployment locks. The result is bounded by the existing 25-member ceiling; an over-ceiling or count mismatch fails closed. The invitation history remains capped at 100. This records known accepted identities; it does not inventory accounts created outside these invitations or inspect current native moderation/deactivation status.

The browser renders accepted members separately and rejects absent/malformed rosters, duplicate identities/references or a count mismatch. A denied or failed summary read removes both roster and invitation codes. The new browser requires the matching API response; older APIs without `accounts` pause these controls rather than inventing a complete list. No schema, permission or native mutation is added.

A PostgreSQL regression fixture places one accepted invitation behind 105 newer expired invitations. The owner and an explicitly enabled current administrator still receive the accepted member, while only the newest 100 invitations are returned; foreign and revoked access is refused. Node contract checks cover independent identity validation, duplicate references and mismatched counts. Controlled browser responses displayed an accepted member absent from all 100 history rows, cleared it on denied/malformed data, recovered, and showed the empty state. The panel fit 320, 768 and 1280 CSS pixels without horizontal overflow. Private browser evidence: `.local/member-roster-browser.json`.

Full pinned smoke passed: 47 Node tests, frontend build, Go vet and all Go race packages. The qualification PostgreSQL retained its volume, had zero generated test databases and was stopped. Hosted PR/main CI and deployment remain separate proof states.
