# Roadmap for standalone managed PDS hosting

Status: **M0 complete; M1–M5 remain open.** Updated October 20, 2026 (October 21 UTC). The live issue audit found 13 already closed issues and 27 open issues; none of the remaining issues has complete closure evidence. The [acceptance audit](issue-acceptance-audit-2026-10-20.md) names the remaining gate for each open issue. Completed checkboxes below reflect hosted issue state, not an estimate of implementation percentage.

A public homelab pilot has recorded real account/email, signed records/media and downstream federation evidence. General managed provisioning and public launch remain unqualified. The owner selected the homelab as an interim private pilot until a VPS is available, while retaining M4's independent production/recovery requirement.

The product is a standalone service with browser onboarding and an optional later CLI. Earlier application-suite research does not define this roadmap. No calendar promises are assigned before the first runnable slice establishes delivery speed.

Each issue is a bounded demonstrable behavior with its own acceptance and verification. Dependencies identify actual blockers; milestone order alone is not an additional blocker. `ready-for-agent` means scoped for implementation, not permission to ignore native blocking links. P0 work is required for its release gate, P1 completes the supported experience, and P2 is later expansion.

## Phases and exit gates

### M0 - Runnable foundation

Local browser/API/database and PDS fixtures run reproducibly; staging infrastructure and resumable deployment intent are ready.

Complete: all six M0 issues are accepted and closed. Production hosting and paid provisioning are outside this foundation milestone.

- [x] [#1: Run the standalone local stack and CI smoke journey](https://git.subcult.tv/subculture-collective/prichal/issues/1)
- [x] [#2: Sign in and create an owner-scoped organization](https://git.subcult.tv/subculture-collective/prichal/issues/2)
- [x] [#3: Bootstrap and register a staging hosting node](https://git.subcult.tv/subculture-collective/prichal/issues/3)
- [x] [#4: Qualify a disposable official PDS runtime](https://git.subcult.tv/subculture-collective/prichal/issues/4)
- [x] [#5: Submit and resume a durable deployment request](https://git.subcult.tv/subculture-collective/prichal/issues/5)
- [x] [#128: Separate the dev.sh enrollment key variables](https://git.subcult.tv/subculture-collective/prichal/issues/128)

### M1 - Private deployment alpha

An invited owner deploys a real managed-domain PDS, creates an account, verifies federation and configures a BYO handle.

- [ ] [#6: Provision a real PDS through the restricted host agent](https://git.subcult.tv/subculture-collective/prichal/issues/6)
- [ ] [#7: Expose stable managed PDS hostnames with HTTPS](https://git.subcult.tv/subculture-collective/prichal/issues/7)
- [ ] [#8: Guide the owner through first-account enrollment](https://git.subcult.tv/subculture-collective/prichal/issues/8)
- [x] [#9: Prove signed records, media and downstream federation](https://git.subcult.tv/subculture-collective/prichal/issues/9)
- [ ] [#10: Attach and verify a customer-owned domain handle](https://git.subcult.tv/subculture-collective/prichal/issues/10)
- [x] [#125: Fix edge and guard bind-mount permissions on native Linux Docker](https://git.subcult.tv/subculture-collective/prichal/issues/125)
- [ ] [#126: Choose the PDS hostname and handle domain](https://git.subcult.tv/subculture-collective/prichal/issues/126)

### M2 - Recoverable managed service

Identity custody, full restores, monitoring, safe upgrades, tenant isolation and operator suspension are demonstrated.

- [ ] [#11: Protect credentials and prove identity-key recovery](https://git.subcult.tv/subculture-collective/prichal/issues/11)
- [ ] [#12: Back up and restore a complete customer PDS](https://git.subcult.tv/subculture-collective/prichal/issues/12)
- [ ] [#13: Restore management state and reconcile recovered jobs](https://git.subcult.tv/subculture-collective/prichal/issues/13)
- [ ] [#14: Detect outages, failed backups and exhausted capacity](https://git.subcult.tv/subculture-collective/prichal/issues/14)
- [ ] [#15: Upgrade a PDS with staged rollout and safe recovery](https://git.subcult.tv/subculture-collective/prichal/issues/15)
- [ ] [#16: Enforce tenant isolation and runtime resource boundaries](https://git.subcult.tv/subculture-collective/prichal/issues/16)
- [ ] [#17: Suspend and resume service through audited operator actions](https://git.subcult.tv/subculture-collective/prichal/issues/17)

### M3 - Paid pilot

Subscription entitlements, usage limits, fleet capacity, exports and support workflows operate through a measured invited pilot.

- [ ] [#18: Gate paid deployment with subscription entitlements](https://git.subcult.tv/subculture-collective/prichal/issues/18)
- [ ] [#19: Show usage and enforce plan limits without surprise loss](https://git.subcult.tv/subculture-collective/prichal/issues/19)
- [ ] [#20: Add capacity and drain a hosting node safely](https://git.subcult.tv/subculture-collective/prichal/issues/20)
- [ ] [#21: Export data and cancel hosting with a safe exit window](https://git.subcult.tv/subculture-collective/prichal/issues/21)
- [ ] [#22: Give operators a tenant support and audit workspace](https://git.subcult.tv/subculture-collective/prichal/issues/22)
- [ ] [#23: Run an invited pilot and measure the deployment target](https://git.subcult.tv/subculture-collective/prichal/issues/23)

### M4 - Public launch

Browser onboarding and independent production infrastructure pass the documented release gate before public signup opens.

The owner confirmed on October 20, 2026 that the homelab is an interim private pilot until a VPS is available. M4's independence requirement remains unchanged. See the [M4 release gate](m4-release-gate.md) for the selected approach, bounded browser evidence and remaining production/launch checks. The [release inventory](m4-release-inventory.md) identifies existing build/configuration artifacts and missing publication, non-local worker and independent recovery evidence.

- [ ] [#24: Finish accessible browser onboarding and lifecycle guidance](https://git.subcult.tv/subculture-collective/prichal/issues/24)
- [ ] [#25: Qualify independent production deployment and incident recovery](https://git.subcult.tv/subculture-collective/prichal/issues/25)
- [ ] [#26: Pass the public-launch gate and open bounded signup](https://git.subcult.tv/subculture-collective/prichal/issues/26)

### M5 - Additional access and portability

Optional CLI, custom PDS endpoints, account migration and organization collaboration extend the qualified API.

- [ ] [#27: Deploy through a downloadable Go CLI](https://git.subcult.tv/subculture-collective/prichal/issues/27)
- [ ] [#28: Deploy on a verified customer-owned PDS endpoint](https://git.subcult.tv/subculture-collective/prichal/issues/28)
- [ ] [#29: Migrate an existing AT account with resumable verification](https://git.subcult.tv/subculture-collective/prichal/issues/29)
- [ ] [#30: Invite organization members with scoped management roles](https://git.subcult.tv/subculture-collective/prichal/issues/30)
- [ ] [#117: Let an organization invite members to create accounts on its PDS](https://git.subcult.tv/subculture-collective/prichal/issues/117)

## Accepted evidence and remaining gates

Completed cross-phase follow-ups are [#127 preview hardening](https://git.subcult.tv/subculture-collective/prichal/issues/127), [#133 strict PR CI context](https://git.subcult.tv/subculture-collective/prichal/issues/133), [#153 native email sign-in codes](https://git.subcult.tv/subculture-collective/prichal/issues/153), [#154 runtime-release observations](https://git.subcult.tv/subculture-collective/prichal/issues/154) and [#155 public-record browsing](https://git.subcult.tv/subculture-collective/prichal/issues/155). These accepted slices do not close their broader launch dependencies.

This snapshot incorporates the October 20 issue audit and browser work through #199, including private/public staging, the real pilot, preview backup execution, native member recovery, native CLI qualification and the recent browser retry/layout fixes. Each linked runbook names its revision, environment and limits. These are dated observations, not a fresh health or running-release assertion. Source main, PR-head CI, merged-main CI, installed preview/pilot releases and production qualification remain distinct.

| Phase | Accepted or recorded evidence | Remaining before the phase closes |
| --- | --- | --- |
| M0 — complete (6/6 issues closed) | Owner-scoped organizations, durable deployment intent, disposable official PDS qualification and [accepted staging bootstrap](live-kvant-staging-node.md). #1–#5 and #128 are closed. | None within M0's accepted scope. General public hosting is later work. |
| M1 — open (2/7 closed) | #125 fixed native Linux readers; #9 accepted [public pilot federation](live-pilot-federation.md). Real pilot account creation and confirmed mail, fixed public TLS and [private worker placement/replay](live-staging-management-connection.md) are recorded. | Non-local management/worker provisioning, general managed namespace/DNS/TLS design (#126/#7), complete managed enrollment/client login and owner-scoped BYO-handle qualification. A fixed pilot is not the general managed product path. |
| M2 — open (0/7 closed) | [Populated pilot restore](live-pilot-recovery.md), [connected staging recovery](live-staging-connection-recovery.md), [native member recovery](local-member-native-recovery.md), [preview backup execution](live-preview-backup-rollout.md), certificate alerts, local isolation and lifecycle checks. | Accepted custody/rotation, node-loss public cutover with new writes/downstream events, full monitoring coverage, storage/resource-pressure controls and successful/failed distinct-release upgrades. The opt-in failed-candidate native path remains unqualified. |
| M3 — open (0/6 closed) | Pilot grants, signed fixture billing inbox, observed usage, capacity admission, portable exports, dated reversible local exit and operator workspace. See [billing inbox](local-billing-inbox.md), [usage](local-usage.md), [customer exports](local-customer-exports.md), [hosting exit](local-hosting-exit.md) and [operator workspace](local-operator-workspace.md). | Paid Checkout/Portal and reconciliation policy, enforced plan/storage limits, populated-node drain, commercial cancellation/retention, notifications and a measured invited pilot. Fixture events are not payments; retention review does not delete data. |
| M4 — open (0/3 closed) | Merged browser work through #199 records bounded retry, focus, layout and request-context checks. [Connected local management](local-connected-management-browser.md) adds real local OIDC/session, organization, River simulation/reload and owner-isolation evidence. [Hosting history](local-owner-journey.md) adds connected read/error/timeout and keyboard pagination checks. See the [M4 release gate](m4-release-gate.md) for #24–#26. | Connected onboarding and real dependency-failure journeys, accessibility progress verification, independent production/recovery, measured service terms, controlled production observation and launch sign-off. Public signup is not qualified. |
| M5 — open (0/5 closed) | [Native Linux CLI runtime/auth/completion/interruption](local-customer-cli.md), [local management roles](local-organization-access.md), [member invitation/removal](local-member-invitations.md), endpoint preflight/choice and migration inventory. | Published/installed and platform-qualified CLI; custom endpoint activation; inbound/outbound identity migration; complete delegated permissions/populated handover; verified organization-domain member handles, plan limits and remaining post-window removal qualification. |

Implementation and test entrypoints include `backend/internal/auth`, `deployment`, `fleet`, `nodeagent`, `recovery`, `observability`, `lifecycle`, `billing`, `exports`, `support` and `orgaccess`. Each feature package has focused tests. `web/src` contains the corresponding browser controls; `scripts/qualify-*.mjs` exercise bounded disposable local workflows. [Progress](progress/local-m1-alpha.md) records exact drills, retained state, failures and gate results. Source files and passing builds alone do not prove the phase exit gates.

Management-owner creation can now be paused independently of deployment/enrollment. Existing owner sign-in and scoped access remain available; local close/reopen and schema-27 recovery passed. See [owner admission](local-owner-admission.md). Public launch and production operator controls remain open.

## Next work and explicit decisions

1. Continue M4's connected browser journeys and failure/accessibility qualification, using the [release gate](m4-release-gate.md) and [per-issue acceptance audit](issue-acceptance-audit-2026-10-20.md). Preserve recent component evidence without treating it as full end-to-end acceptance.
2. Advance the managed-product prerequisites in [#129](https://git.subcult.tv/subculture-collective/prichal/issues/129). Its original no-real-PDS premise is superseded by the public pilot; its general provisioning, namespace, enrollment and custody/recovery steps remain unfinished.
3. Record the general managed PDS/default-handle namespace under [#126](https://git.subcult.tv/subculture-collective/prichal/issues/126). The existing pilot hostname and handle do not choose it.
4. Prepare independent production/recovery for the eventual VPS. Provider/account, region, spend ceiling, production issuer/mail ownership and independent backup custody remain explicit choices. No purchase, production deployment or public change is authorized by this roadmap.
5. Keep production quotas and plan/commercial terms in #16/#18/#19. The closed #4 disposable qualification keeps its existing ceiling; it does not require production total-disk quotas.

Do not delete retained tenants or recovery storage to free occupied qualification slots. The member-removal migration window has real initial-window evidence, but elapsed-window and combined live qualification remain separate. Keep optional M5 breadth behind the current M4 objective.

The 15-minute target still needs the stated fresh managed-domain pilot attempts, all failures/retries and provisioning versus onboarding measurements. Fixture timings and one public pilot do not establish that target.

## Foundation dependency order

The original foundation order started with [PDS-01 / Gitea #1](https://git.subcult.tv/subculture-collective/prichal/issues/1). That runnable foundation supports local PDS qualification, identity/organization onboarding and the staging-node bootstrap. PDS-05 establishes durable deployment intent. PDS-06 connects those pieces into restricted provisioning. Work can branch wherever blockers permit it; avoid inventing a serial dependency between independent tests or infrastructure tasks.

M0 proves runnable foundations. M1 proves new-account deployment and BYO handles with invited testing. M2 must pass before hosting customer identities commercially. M3 proves paid service operation and a recoverable exit. M4 is the public-signup gate. M5 features are optional extensions, and none gates the first public launch.

## Release evidence

A phase closes only after its issues satisfy acceptance checks and linked demonstrations. CI passing is necessary where applicable, but does not prove a live PDS, account migration, identity recovery, billing reconciliation or successful restore. Preserve release versions, environment, operation identifiers and secret-free evidence.

The 15-minute managed-domain target must be measured through M3. Report all attempts, failures and retries, distinguish endpoint readiness from first-account completion, and state DNS/identity/downstream-service assumptions. Custom endpoint setup and migration have their own measurements.

## Scope and decisions

Default to one region and prepared shared hosts with isolated PDS containers. Provider, account budget, purchased domains, identity issuer, exact plans/limits, key custody, support hours and retention remain explicit issue-level decisions. No Kubernetes, independent relay network, custom AT implementation, existing-application integration or mandatory npm distribution is required for the first launch.

[Implementation plan](implementation-plan.md) describes architecture, failure behavior and recovery. [Issue index](issues/README.md) provides the complete backlog and blockers.

## Supporting qualification reports

[Endpoint claims](local-exclusive-endpoint-claims.md), [browser claim review](local-endpoint-claim-workspace.md) and [populated claim recovery](local-populated-claim-recovery.md) record local reservation and custody checks. Public activation remains a separate #28 gate.

Fixture billing has [preparation](local-checkout-preparation.md), [dispatch](local-checkout-dispatch.md), [owner routes](local-checkout-owner-routes.md), [signed intake](local-checkout-completion-inbox.md), [completion mapping](local-checkout-completion-mapping.md) and [actual synthetic authority](local-checkout-completion-authority.md) evidence. [Hosted fixture review](local-checkout-hosted-review.md) and [checkout workspace/layout checks](local-checkout-workspace.md) cover bounded browser behavior. These results do not establish actual Stripe qualification or accepted commercial entitlement policy.

Use the [dated progress log](progress/local-m1-alpha.md) for individual drills and retained evidence. Use the [acceptance audit](issue-acceptance-audit-2026-10-20.md) for current closure decisions; do not treat an older runbook's remaining-work paragraph as the latest project status.
