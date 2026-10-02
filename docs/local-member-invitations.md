# Local member invitations

Issue [#117](https://git.subcult.tv/PatrickFanella/pds-ass/issues/117). An owner whose first account exists can invite members to create their own native accounts on the same PDS. Each invitation is one native single-use code. Members enter their password and email directly on the PDS; management never receives them. This is the first of two deliveries: browser controls, member removal and administrator permission follow separately.

## Owner routes

| Route | Purpose |
|---|---|
| `POST /api/deployments/{id}/members/invitations` | Request an invitation with `{"idempotencyKey": UUID}`. A repeated key returns the original invitation. |
| `GET /api/deployments/{id}/members/invitations` | Ceiling, accepted member count, open invitations and the newest 100 invitations. |
| `GET /api/member-invitations/{id}` | One invitation with its code while open, and its history. |
| `POST /api/member-invitations/{id}/refresh` | Ask the node whether the code was used. At most once every five seconds. |
| `POST /api/member-invitations/{id}/withdraw` | Disable an unused open code. A request still pending returns its current status; withdraw it once it is ready. |

Routes are local-only and owner-only, with the same session, origin and CSRF checks as first-account enrollment. A new request needs a ready agent deployment, a recorded first account, open enrollment admission, no recovery hold and no unreversed hosting exit. Each deployment allows ten new requests per hour. Pausing enrollment admission pauses member requests too; open invitations stay valid until they expire.

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

`node scripts/qualify-member-invitations.mjs DEPLOYMENT_UUID` drives the whole journey through the owner API, worker, agent, guard and native signup. It has not run. On native Linux Docker the local ingress and admission guard cannot read their owner-only bind mounts: they run as container root without capabilities, and the agent's files belong to the host user. Caddy exits and the guard denies every signup, first-account or member. Earlier local drills ran on Docker Desktop, which maps bind-mount ownership. This predates #117 and needs its own fix before the full journey can run here.

Not covered: browser controls, member removal and the 30-day migration window, administrator permission, organization-domain handles (#10), live expiry of a real 72-hour code, public DNS, TLS, email and federation.
