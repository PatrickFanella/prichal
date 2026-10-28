# M4 release gate

M4 remains the public-launch milestone. Its acceptance requires independent production serving and recovery, complete browser journeys and a reviewed launch evidence packet. Local checks, merged CI and homelab pilot results contribute evidence but do not complete those requirements by themselves.

The live acceptance criteria are [#24](https://git.subcult.tv/subculture-collective/prichal/issues/24), [#25](https://git.subcult.tv/subculture-collective/prichal/issues/25) and [#26](https://git.subcult.tv/subculture-collective/prichal/issues/26). This checkpoint records the owner's environment decision and organizes the remaining evidence. It does not change those criteria or close an issue.

## Environment decision — October 20, 2026

The owner selected this completion target: **keep M4's independence requirement; use the homelab as an interim private pilot until a VPS is available.**

Continue invited pilot qualification and release preparation on the homelab within its existing authorization. Preserve retained tenants, keys, backups and rollback records. A production environment must eventually serve and recover with the homelab and development workstation unavailable. Buying a VPS alone does not establish that capability.

The provider/account, region, spend ceiling, general managed-domain namespace, production identity/mail ownership and independent backup custody remain unselected. The existing pilot domain does not select the general hosting namespace. This decision authorizes no purchase, production deployment, DNS cutover or public-signup opening.

Public signup must remain closed while a critical launch gate is incomplete. Keep the interim pilot private; do not rename it production qualification to complete M4.

## Browser evidence and remaining work — #24

The following combine bounded component checks with a connected local management qualification. They do not complete the supported end-to-end onboarding journey:

| Area | Recorded evidence | Remaining acceptance |
| --- | --- | --- |
| Workspace sign-in and refresh | [Owner workspace context](local-owner-workspace-context.md): sign-out/read races, superseded 401, confirmed-creation/read-failure guidance and manual read recovery. [Connected local management](local-connected-management-browser.md): local OIDC sign-in, real sessions, creation and Alice/Bob isolation. | External identity-provider revocation and the complete supported journey remain open. |
| Deployment submission and return | [Retry guidance](local-deployment-retry-guidance.md): retained payload, poll failure/recovery, reopening, errors and focused retries. [Connected local management](local-connected-management-browser.md): persisted queued simulation, reload/retry without a duplicate job and real River completion. | Native provisioning and its failures on designated release artifacts remain open. |
| Domain instructions | [Request context](local-domain-instruction-context.md): stale reads/submissions, unsaved edits, read retry and timeout. | Qualify actual domain proof, account ownership and DNS failure recovery; preserve #10 and domain-selection gates. |
| First-account preparation | [Invitation retry guidance](local-enrollment-retry-guidance.md): retained original/recovery payloads, full navigation, keyboard retry and result focus. | Include actual enrollment and ownership boundaries in the complete journey. Controlled responses do not prove invitation delivery or native account creation. |
| Checkout review | [Positive receipt layouts and focus](local-checkout-workspace.md#positive-receipt-layouts-and-focus--october-20-2026): five fixture receipt states at five widths, keyboard review and unavailable-read behavior. | Complete the actual-authority payment journey and its failure recovery. Fixture receipt/session responses do not qualify real provider behavior or commercial entitlement. |
| Usage and exit | [Export request context](local-export-request-context.md), [customer exports](local-customer-exports.md), [hosting exit](local-hosting-exit.md) and [exit status/request context](local-hosting-exit-status-context.md) record distinct local behaviors. | Complete #19/#21 acceptance and the connected usage, capture/download and cancellation journey. Keep commercial policy separate from fixture behavior. |
| Accessibility and layout | The linked browser reports record five desktop-browser CSS viewport sizes, including portrait and landscape. [Connected hosting history](local-owner-journey.md) adds keyboard loading/error/recovery focus, timeout, pagination and late-response checks against the local application and API. | Verify labels, keyboard navigation, visible focus and progress announcements across the supported journey, including DNS, payment and provisioning failures and refreshed/reopened sessions. Component checks do not establish screen-reader speech or physical-device behavior. |

## Independent production and incident recovery — #25

| Gate | Evidence required before acceptance |
| --- | --- |
| Selected environment and ownership | Record provider/account, region, domains and DNS owner, issuer/client and mail ownership, approved spend ceiling, secret custody, backup destination and key/restore custodian. Do not fill unresolved choices with inferred defaults. |
| Environment separation | Verify distinct staging/production credentials, infrastructure state, billing mode and host inventories. Keep management PostgreSQL separate from native PDS SQLite/blob custody. |
| Reproducible release | Record pinned API/web/worker/agent image digests, CI results, versioned configuration, schema/migration gates and health checks for the designated release. Source CI is separate from deployed-image evidence. |
| Recovery and incidents | Use designated disposable tenants for management and PDS restore drills. Recreate the production-design environment and verify auth, billing and mail routing with development/homelab access unavailable. Record actual outcomes and rollback. Homelab backup and isolated restore evidence alone cannot satisfy independence. |
| Operational ownership | Record support/on-call hours, escalation and abuse contacts, dependency-failure procedures and rollback steps. Demonstrate independent serving and recovery rather than relying on an operator workstation. |

Existing [production pilot agent](production-pilot-agent.md), [management recovery](local-recovery.md) and [managed PDS recovery](local-managed-recovery.md) documentation are preparation inputs. Verify their applicability to the selected release and environment; do not carry a local or preview result forward as production evidence. Keep #25's dependencies #3/#11/#13/#14/#15/#18 subject to their own acceptance checks.

## Public-launch acceptance — #26

| Gate | Evidence required before opening signup |
| --- | --- |
| Complete release journey | Demonstrate managed-domain deployment, account creation and federation, security/isolation and both restore drills on the release artifacts. Include #23's measured invited-pilot target and #24/#25 acceptance. |
| Honest service terms | Confirm plans and enforced limits, support terms, retention/exit behavior, abuse contact and advertised timing against measured capabilities. Fixture billing and observed usage do not establish commercial policy or quotas. |
| Bounded admission | Rehearse signup opening/closing, a normal deployment and capacity exhaustion. Verify an immediate operator closure mechanism while existing owners retain recovery access. |
| Limitations and incidents | List unresolved limitations with named accountable owners. Rehearse a support incident. Any failed critical gate keeps signup closed. |
| Controlled production observation | Obtain the separate deployment authorization, record release versions and observe the first controlled production deployments. Retain a signed-off checklist with evidence links. |

## Evidence packet and next steps

The [source release inventory](m4-release-inventory.md) records current API/web/worker/agent, configuration and backup boundaries, plus the candidate record and deployment/rollback review sequence. It supplies preparation for #25 without selecting infrastructure or claiming deployment qualification.

For each acceptance result, record the issue/criterion, source revision and image digests, environment, UTC observation time, operation identifier, expected and observed behavior, cleanup/rollback outcome and a secret-free evidence link. State whether the result is local validation, PR-head CI, merged-main CI, deployment, runtime observation or qualification. Keep credentials and private evidence in the approved private storage.

Continue useful browser qualification, release preparation and recovery work that does not require an unresolved external choice. Before paid infrastructure or public changes, prepare a concrete release inventory, migration plan, backup/restore evidence, rollback commands and verification checklist for owner review. Request the selected account, spend ceiling and execution authorization only when the proposed action is concrete. Do not alter milestone scope to bypass unavailable infrastructure.
