# Roadmap for standalone managed PDS hosting

Status: planned. Updated September 30, 2026.

The product is a standalone service with browser onboarding and an optional later CLI. Earlier application-suite research does not define this roadmap. No calendar promises are assigned before the first runnable slice establishes delivery speed.

Each issue is a bounded demonstrable behavior with its own acceptance and verification. Dependencies identify actual blockers; milestone order alone is not an additional blocker. `ready-for-agent` means scoped for implementation, not permission to ignore native blocking links. P0 work is required for its release gate, P1 completes the supported experience, and P2 is later expansion.

## Phases and exit gates

### M0 - Runnable foundation

Local browser/API/database and PDS fixtures run reproducibly; staging infrastructure and resumable deployment intent are ready.

- [PDS-01: Run the standalone local stack and CI smoke journey](issues/01-run-the-standalone-local-stack-and-ci-smoke-journey.md)
- [PDS-02: Sign in and create an owner-scoped organization](issues/02-sign-in-and-create-an-owner-scoped-organization.md)
- [PDS-03: Bootstrap and register a staging hosting node](issues/03-bootstrap-and-register-a-staging-hosting-node.md)
- [PDS-04: Qualify a disposable official PDS runtime](issues/04-qualify-a-disposable-official-pds-runtime.md)
- [PDS-05: Submit and resume a durable deployment request](issues/05-submit-and-resume-a-durable-deployment-request.md)

### M1 - Private deployment alpha

An invited owner deploys a real managed-domain PDS, creates an account, verifies federation and configures a BYO handle.

- [PDS-06: Provision a real PDS through the restricted host agent](issues/06-provision-a-real-pds-through-the-restricted-host-agent.md)
- [PDS-07: Expose stable managed PDS hostnames with HTTPS](issues/07-expose-stable-managed-pds-hostnames-with-https.md)
- [PDS-08: Guide the owner through first-account enrollment](issues/08-guide-the-owner-through-first-account-enrollment.md)
- [PDS-09: Prove signed records, media and downstream federation](issues/09-prove-signed-records-media-and-downstream-federation.md)
- [PDS-10: Attach and verify a customer-owned domain handle](issues/10-attach-and-verify-a-customer-owned-domain-handle.md)

### M2 - Recoverable managed service

Identity custody, full restores, monitoring, safe upgrades, tenant isolation and operator suspension are demonstrated.

- [PDS-11: Protect credentials and prove identity-key recovery](issues/11-protect-credentials-and-prove-identity-key-recovery.md)
- [PDS-12: Back up and restore a complete customer PDS](issues/12-back-up-and-restore-a-complete-customer-pds.md)
- [PDS-13: Restore management state and reconcile recovered jobs](issues/13-restore-management-state-and-reconcile-recovered-jobs.md)
- [PDS-14: Detect outages, failed backups and exhausted capacity](issues/14-detect-outages-failed-backups-and-exhausted-capacity.md)
- [PDS-15: Upgrade a PDS with staged rollout and safe recovery](issues/15-upgrade-a-pds-with-staged-rollout-and-safe-recovery.md)
- [PDS-16: Enforce tenant isolation and runtime resource boundaries](issues/16-enforce-tenant-isolation-and-runtime-resource-boundaries.md)
- [PDS-17: Suspend and resume service through audited operator actions](issues/17-suspend-and-resume-service-through-audited-operator-actions.md)

### M3 - Paid pilot

Subscription entitlements, usage limits, fleet capacity, exports and support workflows operate through a measured invited pilot.

- [PDS-18: Gate paid deployment with subscription entitlements](issues/18-gate-paid-deployment-with-subscription-entitlements.md)
- [PDS-19: Show usage and enforce plan limits without surprise loss](issues/19-show-usage-and-enforce-plan-limits-without-surprise-loss.md)
- [PDS-20: Add capacity and drain a hosting node safely](issues/20-add-capacity-and-drain-a-hosting-node-safely.md)
- [PDS-21: Export data and cancel hosting with a safe exit window](issues/21-export-data-and-cancel-hosting-with-a-safe-exit-window.md)
- [PDS-22: Give operators a tenant support and audit workspace](issues/22-give-operators-a-tenant-support-and-audit-workspace.md)
- [PDS-23: Run an invited pilot and measure the deployment target](issues/23-run-an-invited-pilot-and-measure-the-deployment-target.md)

### M4 - Public launch

Browser onboarding and independent production infrastructure pass the documented release gate before public signup opens.

- [PDS-24: Finish accessible browser onboarding and lifecycle guidance](issues/24-finish-accessible-browser-onboarding-and-lifecycle-guidance.md)
- [PDS-25: Qualify independent production deployment and incident recovery](issues/25-qualify-independent-production-deployment-and-incident-recovery.md)
- [PDS-26: Pass the public-launch gate and open bounded signup](issues/26-pass-the-public-launch-gate-and-open-bounded-signup.md)

### M5 - Additional access and portability

Optional CLI, custom PDS endpoints, account migration and organization collaboration extend the qualified API.

- [PDS-27: Deploy through a downloadable Go CLI](issues/27-deploy-through-a-downloadable-go-cli.md)
- [PDS-28: Deploy on a verified customer-owned PDS endpoint](issues/28-deploy-on-a-verified-customer-owned-pds-endpoint.md)
- [PDS-29: Migrate an existing AT account with resumable verification](issues/29-migrate-an-existing-at-account-with-resumable-verification.md)
- [PDS-30: Invite organization members with scoped management roles](issues/30-invite-organization-members-with-scoped-management-roles.md)

## First work and dependency order

Start PDS-01. Its completion unblocks local PDS qualification, identity/organization onboarding and the staging-node bootstrap. PDS-05 then establishes durable deployment intent. PDS-06 connects those pieces into real provisioning. Work can branch wherever blockers permit it; avoid inventing a serial dependency between independent tests or infrastructure tasks.

M0 proves runnable foundations. M1 proves new-account deployment and BYO handles with invited testing. M2 must pass before hosting customer identities commercially. M3 proves paid service operation and a recoverable exit. M4 is the public-signup gate. M5 features are optional extensions, and none gates the first public launch.

## Release evidence

A phase closes only after its issues satisfy acceptance checks and linked demonstrations. CI passing is necessary where applicable, but does not prove a live PDS, account migration, identity recovery, billing reconciliation or successful restore. Preserve release versions, environment, operation identifiers and secret-free evidence.

The 15-minute managed-domain target must be measured through M3. Report all attempts, failures and retries, distinguish endpoint readiness from first-account completion, and state DNS/identity/downstream-service assumptions. Custom endpoint setup and migration have their own measurements.

## Scope and decisions

Default to one region and prepared shared hosts with isolated PDS containers. Provider, account budget, purchased domains, identity issuer, exact plans/limits, key custody, support hours and retention remain explicit issue-level decisions. No Kubernetes, independent relay network, custom AT implementation, existing-application integration or mandatory npm distribution is required for the first launch.

[Implementation plan](implementation-plan.md) describes architecture, failure behavior and recovery. [Issue index](issues/README.md) provides the complete backlog and blockers.
