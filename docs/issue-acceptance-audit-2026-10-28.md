# Issue acceptance audit — October 28, 2026

Reviewed source main `f2c73e135166` and all 40 hosted issues: **13 closed, 27 open**. Refreshed every open issue's acceptance body, comments and native blocking edges. Compared these with the [previous audit](issue-acceptance-audit-2026-10-20.md), subsequent merged changes, delivery reports and relevant code gates. Private API snapshots are in `.local/issue-audit/2026-10-28/`.

No additional issue has complete acceptance evidence. None was closed. Already closed: #1–#5, #9, #125, #127, #128, #133, #153, #154 and #155. Historical runtime reports retain their original revision and environment; this audit does not reverify running services.

Recent browser merges support #24 without completing its connected enrollment/domain/payment/exit and accessibility journey. The release inventory for #25 identifies missing artifacts and infrastructure decisions; it does not supply a production release. Native dependencies remain unchanged.

## Remaining gates for every open issue

| Issue | Completed or recorded work | Remaining acceptance or dependency |
| --- | --- | --- |
| #6 — restricted provisioning | [Local worker provisioning](local-agent-provisioning.md), [private staging placement/replay](live-staging-management-connection.md) and a [fixed public pilot agent](production-pilot-agent.md). | General non-local management/worker provisioning and interrupted-job staging qualification remain open. The worker still requires explicit local mode; host-controller pilot provisioning is narrower than this issue. |
| #7 — managed HTTPS hostnames | [Fixed public pilot](live-dozor-pilot.md), [Kvant public staging](live-kvant-public-pds.md) and certificate qualification. | General managed namespace decision, DNS/TLS automation and failure/retry qualification remain; #6 is an open native dependency. A fixed pilot domain does not resolve #126. |
| #8 — first account | Real pilot account creation and owner-confirmed email are recorded in [pilot evidence](live-dozor-pilot.md); [local enrollment](local-owner-enrollment.md) covers bounded failure paths. | Production management enrollment and compatible-client/browser login acceptance remain; #7 is open. |
| #10 — customer handle | [Public pilot handle](live-pilot-federation.md), [local handle proof](local-handle-proof.md) and [browser request-context checks](local-domain-instruction-context.md). | Complete the owner-scoped managed journey and controlled failure/removal checks against its designated environment; #8 is open. |
| #11 — credentials and identity recovery | Protected pilot/staging custody, retained-key certificate renewal and [encrypted recovery](live-pilot-recovery.md). | Complete the accepted custody and credential-rotation policy and recovery verification. Pilot SMTP policy mutation is not implemented; configuration encryption alone is insufficient. |
| #12 — complete PDS restore | [Populated pilot offline restore](live-pilot-recovery.md), [managed recovery](local-managed-recovery.md) and [native member recovery](local-member-native-recovery.md). | Node-loss recovery must retain endpoint/identity, fence writers, prove new writes/downstream events and measure the stated recovery targets. #6/#11 remain open. An isolated offline restore is not a public cutover drill. |
| #13 — management recovery | [Management recovery](local-recovery.md), [connected staging restore](live-staging-connection-recovery.md) and later member/checkout reconciliation evidence. | #11 remains an open native dependency. Complete accepted custody/rotation and the remaining designated-environment recovery acceptance; do not discard the completed local restore evidence. |
| #14 — monitoring | [Local monitoring](local-monitoring.md), [public certificate alerts](live-kvant-public-pds-alerts.md) and [natural preview backup execution](live-preview-backup-rollout.md). | Complete hosting-network-independent probe and incident/alert coverage for the supported service. #7/#12/#13 remain open. Preview scheduling does not establish all-tenant backup coverage. |
| #15 — safe upgrades | [Release rehearsal](local-release-rehearsal.md), pinned release observations and retained-source recovery tooling. | Qualify successful and failed distinct-release upgrades with populated data and compatible recovery. The native failed-candidate path remains unqualified; #12 is open. |
| #16 — isolation and resources | [Two-tenant isolation checks](local-tenant-isolation.md), configured CPU/memory/process ceilings and disposable storage failure qualification. | Total storage controls, supported resource-pressure/noisy-neighbor behavior and remaining live network boundaries are unqualified. #6/#7/#11 remain open. |
| #17 — suspend/resume | Audited local service/account actions and [operator lifecycle workspace](local-operator-lifecycle-workspace.md). | #14/#16 remain open; the issue requires qualified lifecycle restrictions, retention and review/resumption behavior across the supported environment. |
| #18 — subscription entitlement | Fixture checkout, signed intake and reconciliation with explicit pilot grants; see [checkout review](local-checkout-workspace.md). | Select plans/prices/allowances/grace rules and qualify actual Stripe test-mode Checkout/Portal and event ordering/bypass behavior. Fixture authority is not Stripe qualification. |
| #19 — usage and plan limits | [Observed usage/freshness](local-usage.md) and capacity/admission checks. | Enforce selected storage/account/instance allowances, test two plans and downgrade/restore behavior. #16/#18 are open. Observed bytes are not an enforced quota. |
| #20 — capacity and drain | [Capacity policy](local-capacity-policy.md) and retained placement accounting. | Qualify populated-node drain and capacity transitions without identity/data loss; #6/#12/#19 are open. |
| #21 — export and exit | [Portable exports](local-customer-exports.md), [reversible local hosting exit](local-hosting-exit.md) and [export retry context](local-export-request-context.md). | Complete accepted commercial cancellation/retention and the connected exit journey; #12/#17/#18 are open. Local retention review does not delete data or cancel payment. |
| #22 — operator support | [Scoped operator workspace](local-operator-workspace.md), redaction and operation history. | Qualified lifecycle/billing actions are prerequisites; #17/#18 remain open. A local support view does not independently complete those actions' acceptance. |
| #23 — measured invited pilot | A real disposable pilot and recovery reports exist. | At least 20 documented fresh managed-domain attempts, complete timing/failure reporting, accepted paid cohort and cost/support measurements remain. Native dependencies #10/#14/#15/#16/#18/#19/#20/#21/#22 are open. |
| #24 — browser onboarding | Merged retry/context fixes, [connected local OIDC/organization/deployment checks](local-connected-management-browser.md) and [connected history failure/timeout/pagination checks](local-owner-journey.md); see the [M4 evidence map](m4-release-gate.md). | Complete connected supported journeys, real dependency failure/recovery and accessibility progress verification. #10/#19/#21 remain open. Component and CSS viewport checks are bounded evidence. |
| #25 — independent production | Homelab pilot, staging recovery and the [source release inventory](m4-release-inventory.md); the owner explicitly retained M4 independence. | VPS/account/region/budget and custody choices, independent serving/recovery, release and incident qualification remain. #11/#13/#14/#15/#18 are open. |
| #26 — public launch | Local admission controls and a [release checklist](m4-release-gate.md) exist. | #23/#24/#25, accepted service terms, production observation and release sign-off remain. Keep public signup gated. |
| #27 — CLI | [Native Linux runtime, completion, interruption and real local device-auth checks](local-customer-cli.md); five build targets. | Publish and install supported release binaries and qualify the full authorize/deploy/reconnect flow on supported systems. Build artifacts and loopback process tests alone do not complete installation/runtime acceptance. |
| #28 — custom PDS endpoint | [Custom-address preflight](local-custom-endpoint-preflight.md), retained choice and DNS observation. | Controlled public activation, certificate failure and endpoint-preserving recovery remain; #7/#11 are open. A stored or observed choice is not activation. |
| #29 — account migration | [Retained-export migration inventory](local-migration-preflight.md) and browser inspection controls. | Inbound/outbound populated-host transfer, interruption, explicit identity switch and post-switch/rollback boundaries remain; #12/#21 are open. |
| #30 — management roles | [Identity-bound local collaboration](local-organization-access.md), revocation, bounded history and empty-organization handover. | Delegated hosting/domain permissions, populated handover and live identity/provider qualification remain; #22 is open. |
| #117 — organization accounts | [Member invitations](local-member-invitations.md), administrator opt-in, roster, durable removal and retained migration window. | Verified organization-domain handles, plan-linked limits, combined live administrator/native qualification and post-deadline management behavior remain. The 25-member placeholder is not a selected plan. |
| #126 — namespace decision | Options and fixed pilot identities are documented. | Record the selected general PDS/default-handle domain, DNS ownership, wildcard/TLS design and reserved-name/abuse policy. The homelab-first decision selected none of these. |
| #129 — real-PDS delivery order | The original no-live-PDS premise is superseded by the public pilot and closed #9. | Its ordered managed-provisioning, namespace, enrollment and custody/recovery work still includes open #6/#7/#8/#11/#12/#126. Preserve this distinction rather than describing the pilot as nonexistent or the entire delivery track as done. |

## Next issue: #6 restricted provisioning

Start with #6, whose foundation blockers #3/#4/#5 are closed. It is the earliest unfinished managed-deployment step in #129 and blocks the general M1 path. #126 still needs the owner's namespace decision, but that decision does not prevent a bounded worker configuration fix.

The first change validates the worker's CA/certificate/key tuple before database connection, migrations or queue startup. Previously a certificate or key without the CA setting was silently ignored, leaving agent workers unconfigured. Every partial tuple now fails with an explicit configuration error; a complete tuple uses the existing private-file and certificate validation. Empty transport configuration still supports local simulation.

Regression tests cover all six partial tuples, invalid configured files before database startup, empty simulation configuration and refusal of non-local modes. This is preparation for #6, not completion of its staging verification.

At the reviewed base, the remaining implementation crossed these boundaries (the subsequent control-path update below addresses the worker/admission opt-in):

- [Worker](../backend/cmd/worker/main.go): local-only mode, local billing/export settings and shared queue consumers.
- [Deployment routes](../backend/internal/deployment/http.go) and [store](../backend/internal/deployment/store.go): local-only admission, region and generated `.localhost` identity.
- [Placement/reconciliation](../backend/internal/deployment/agent.go): local node selection and local endpoint validation.
- [Agent client](../backend/internal/nodeagent/client.go): loopback-only peers, pinned identity and local response endpoint validation.

A later non-local path must carry an explicit environment/endpoint policy through these layers, preserve existing test-job semantics and recovery fences, and qualify real interrupted-worker reconciliation on designated staging infrastructure. Removing the mode guard alone would bypass these unresolved contracts. Public namespace/TLS and full production acceptance remain in #7/#126/#25.

## Verification scope

The application change is limited to worker credential startup. The local smoke gate and its exact outcome are recorded in [progress](progress/local-m1-alpha.md). No deployment, identity mutation, DNS change, payment, public signup or retained tenant change was performed. Local tests do not establish non-local staging or production qualification.

## Subsequent #6 implementation

The [private staging control path](private-staging-provisioning.md) adds production-mode opt-in and an agent-only queue after this audit. It preserves local endpoint/placement policy for the private staging environment. Its automated queue/admission checks do not close #6: current Kvant inspection found its one authorized slot occupied, so fresh native interrupted-worker qualification awaits designated capacity. No issue has been closed.

## Source integration and acceptance update

PR #202 merged as `143f109a0d572da272de2cbc9bc93540b44cc3ef` after its required PR-head check passed, and #6 was closed with the accepted private native drill and explicit limits. The issue count is now 14 closed and 26 open. The original table above records the earlier source audit; #6 is no longer an open dependency in those rows. #7's managed public DNS/TLS, #8's enrollment and later recovery/launch gates remain open. See the [current roadmap](roadmap.md) and [#7 implementation sequence](managed-https-design.md).
