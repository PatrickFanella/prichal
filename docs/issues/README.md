# Implementation issues

Status: drafted for publication to [Gitea](https://git.subcult.tv/PatrickFanella/pds-ass/issues).

| Plan ID | Work | Phase | Priority | Blocked by |
| --- | --- | --- | --- | --- |
| PDS-01 | [Run the standalone local stack and CI smoke journey](01-run-the-standalone-local-stack-and-ci-smoke-journey.md) | M0 | P0 | None |
| PDS-02 | [Sign in and create an owner-scoped organization](02-sign-in-and-create-an-owner-scoped-organization.md) | M0 | P0 | PDS-01 |
| PDS-03 | [Bootstrap and register a staging hosting node](03-bootstrap-and-register-a-staging-hosting-node.md) | M0 | P0 | PDS-01 |
| PDS-04 | [Qualify a disposable official PDS runtime](04-qualify-a-disposable-official-pds-runtime.md) | M0 | P0 | PDS-01 |
| PDS-05 | [Submit and resume a durable deployment request](05-submit-and-resume-a-durable-deployment-request.md) | M0 | P0 | PDS-02 |
| PDS-06 | [Provision a real PDS through the restricted host agent](06-provision-a-real-pds-through-the-restricted-host-agent.md) | M1 | P0 | PDS-03, PDS-04, PDS-05 |
| PDS-07 | [Expose stable managed PDS hostnames with HTTPS](07-expose-stable-managed-pds-hostnames-with-https.md) | M1 | P0 | PDS-06 |
| PDS-08 | [Guide the owner through first-account enrollment](08-guide-the-owner-through-first-account-enrollment.md) | M1 | P0 | PDS-02, PDS-07 |
| PDS-09 | [Prove signed records, media and downstream federation](09-prove-signed-records-media-and-downstream-federation.md) | M1 | P0 | PDS-08 |
| PDS-10 | [Attach and verify a customer-owned domain handle](10-attach-and-verify-a-customer-owned-domain-handle.md) | M1 | P1 | PDS-08 |
| PDS-11 | [Protect credentials and prove identity-key recovery](11-protect-credentials-and-prove-identity-key-recovery.md) | M2 | P0 | PDS-03, PDS-04 |
| PDS-12 | [Back up and restore a complete customer PDS](12-back-up-and-restore-a-complete-customer-pds.md) | M2 | P0 | PDS-06, PDS-11 |
| PDS-13 | [Restore management state and reconcile recovered jobs](13-restore-management-state-and-reconcile-recovered-jobs.md) | M2 | P0 | PDS-05, PDS-11 |
| PDS-14 | [Detect outages, failed backups and exhausted capacity](14-detect-outages-failed-backups-and-exhausted-capacity.md) | M2 | P0 | PDS-07, PDS-12, PDS-13 |
| PDS-15 | [Upgrade a PDS with staged rollout and safe recovery](15-upgrade-a-pds-with-staged-rollout-and-safe-recovery.md) | M2 | P0 | PDS-09, PDS-12 |
| PDS-16 | [Enforce tenant isolation and runtime resource boundaries](16-enforce-tenant-isolation-and-runtime-resource-boundaries.md) | M2 | P0 | PDS-06, PDS-07, PDS-11 |
| PDS-17 | [Suspend and resume service through audited operator actions](17-suspend-and-resume-service-through-audited-operator-actions.md) | M2 | P0 | PDS-14, PDS-16 |
| PDS-18 | [Gate paid deployment with subscription entitlements](18-gate-paid-deployment-with-subscription-entitlements.md) | M3 | P0 | PDS-02, PDS-05 |
| PDS-19 | [Show usage and enforce plan limits without surprise loss](19-show-usage-and-enforce-plan-limits-without-surprise-loss.md) | M3 | P0 | PDS-16, PDS-18 |
| PDS-20 | [Add capacity and drain a hosting node safely](20-add-capacity-and-drain-a-hosting-node-safely.md) | M3 | P0 | PDS-06, PDS-12, PDS-19 |
| PDS-21 | [Export data and cancel hosting with a safe exit window](21-export-data-and-cancel-hosting-with-a-safe-exit-window.md) | M3 | P0 | PDS-12, PDS-17, PDS-18 |
| PDS-22 | [Give operators a tenant support and audit workspace](22-give-operators-a-tenant-support-and-audit-workspace.md) | M3 | P1 | PDS-17, PDS-18 |
| PDS-23 | [Run an invited pilot and measure the deployment target](23-run-an-invited-pilot-and-measure-the-deployment-target.md) | M3 | P0 | PDS-09, PDS-10, PDS-14, PDS-15, PDS-16, PDS-18, PDS-19, PDS-20, PDS-21, PDS-22 |
| PDS-24 | [Finish accessible browser onboarding and lifecycle guidance](24-finish-accessible-browser-onboarding-and-lifecycle-guidance.md) | M4 | P1 | PDS-10, PDS-19, PDS-21 |
| PDS-25 | [Qualify independent production deployment and incident recovery](25-qualify-independent-production-deployment-and-incident-recovery.md) | M4 | P0 | PDS-03, PDS-11, PDS-13, PDS-14, PDS-15, PDS-18 |
| PDS-26 | [Pass the public-launch gate and open bounded signup](26-pass-the-public-launch-gate-and-open-bounded-signup.md) | M4 | P0 | PDS-23, PDS-24, PDS-25 |
| PDS-27 | [Deploy through a downloadable Go CLI](27-deploy-through-a-downloadable-go-cli.md) | M5 | P2 | PDS-02, PDS-05, PDS-09 |
| PDS-28 | [Deploy on a verified customer-owned PDS endpoint](28-deploy-on-a-verified-customer-owned-pds-endpoint.md) | M5 | P2 | PDS-07, PDS-11 |
| PDS-29 | [Migrate an existing AT account with resumable verification](29-migrate-an-existing-at-account-with-resumable-verification.md) | M5 | P2 | PDS-09, PDS-12, PDS-21 |
| PDS-30 | [Invite organization members with scoped management roles](30-invite-organization-members-with-scoped-management-roles.md) | M5 | P2 | PDS-02, PDS-22 |

One issue per demonstrable behavior. Local ticket files preserve the initial specification; the Gitea issue is the active execution record after publication.
