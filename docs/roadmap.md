# Roadmap for standalone managed PDS hosting

Status: planned; 30 implementation issues published across six Gitea milestones. Updated September 30, 2026.

The product is a standalone service with browser onboarding and an optional later CLI. Earlier application-suite research does not define this roadmap. No calendar promises are assigned before the first runnable slice establishes delivery speed.

Each issue is a bounded demonstrable behavior with its own acceptance and verification. Dependencies identify actual blockers; milestone order alone is not an additional blocker. `ready-for-agent` means scoped for implementation, not permission to ignore native blocking links. P0 work is required for its release gate, P1 completes the supported experience, and P2 is later expansion.

## Phases and exit gates

### M0 - Runnable foundation

Local browser/API/database and PDS fixtures run reproducibly; staging infrastructure and resumable deployment intent are ready.

- [#1: Run the standalone local stack and CI smoke journey](https://git.subcult.tv/PatrickFanella/pds-ass/issues/1)
- [#2: Sign in and create an owner-scoped organization](https://git.subcult.tv/PatrickFanella/pds-ass/issues/2)
- [#3: Bootstrap and register a staging hosting node](https://git.subcult.tv/PatrickFanella/pds-ass/issues/3)
- [#4: Qualify a disposable official PDS runtime](https://git.subcult.tv/PatrickFanella/pds-ass/issues/4)
- [#5: Submit and resume a durable deployment request](https://git.subcult.tv/PatrickFanella/pds-ass/issues/5)

### M1 - Private deployment alpha

An invited owner deploys a real managed-domain PDS, creates an account, verifies federation and configures a BYO handle.

- [#6: Provision a real PDS through the restricted host agent](https://git.subcult.tv/PatrickFanella/pds-ass/issues/6)
- [#7: Expose stable managed PDS hostnames with HTTPS](https://git.subcult.tv/PatrickFanella/pds-ass/issues/7)
- [#8: Guide the owner through first-account enrollment](https://git.subcult.tv/PatrickFanella/pds-ass/issues/8)
- [#9: Prove signed records, media and downstream federation](https://git.subcult.tv/PatrickFanella/pds-ass/issues/9)
- [#10: Attach and verify a customer-owned domain handle](https://git.subcult.tv/PatrickFanella/pds-ass/issues/10)

### M2 - Recoverable managed service

Identity custody, full restores, monitoring, safe upgrades, tenant isolation and operator suspension are demonstrated.

- [#11: Protect credentials and prove identity-key recovery](https://git.subcult.tv/PatrickFanella/pds-ass/issues/11)
- [#12: Back up and restore a complete customer PDS](https://git.subcult.tv/PatrickFanella/pds-ass/issues/12)
- [#13: Restore management state and reconcile recovered jobs](https://git.subcult.tv/PatrickFanella/pds-ass/issues/13)
- [#14: Detect outages, failed backups and exhausted capacity](https://git.subcult.tv/PatrickFanella/pds-ass/issues/14)
- [#15: Upgrade a PDS with staged rollout and safe recovery](https://git.subcult.tv/PatrickFanella/pds-ass/issues/15)
- [#16: Enforce tenant isolation and runtime resource boundaries](https://git.subcult.tv/PatrickFanella/pds-ass/issues/16)
- [#17: Suspend and resume service through audited operator actions](https://git.subcult.tv/PatrickFanella/pds-ass/issues/17)

### M3 - Paid pilot

Subscription entitlements, usage limits, fleet capacity, exports and support workflows operate through a measured invited pilot.

- [#18: Gate paid deployment with subscription entitlements](https://git.subcult.tv/PatrickFanella/pds-ass/issues/18)
- [#19: Show usage and enforce plan limits without surprise loss](https://git.subcult.tv/PatrickFanella/pds-ass/issues/19)
- [#20: Add capacity and drain a hosting node safely](https://git.subcult.tv/PatrickFanella/pds-ass/issues/20)
- [#21: Export data and cancel hosting with a safe exit window](https://git.subcult.tv/PatrickFanella/pds-ass/issues/21)
- [#22: Give operators a tenant support and audit workspace](https://git.subcult.tv/PatrickFanella/pds-ass/issues/22)
- [#23: Run an invited pilot and measure the deployment target](https://git.subcult.tv/PatrickFanella/pds-ass/issues/23)

### M4 - Public launch

Browser onboarding and independent production infrastructure pass the documented release gate before public signup opens.

- [#24: Finish accessible browser onboarding and lifecycle guidance](https://git.subcult.tv/PatrickFanella/pds-ass/issues/24)
- [#25: Qualify independent production deployment and incident recovery](https://git.subcult.tv/PatrickFanella/pds-ass/issues/25)
- [#26: Pass the public-launch gate and open bounded signup](https://git.subcult.tv/PatrickFanella/pds-ass/issues/26)

### M5 - Additional access and portability

Optional CLI, custom PDS endpoints, account migration and organization collaboration extend the qualified API.

- [#27: Deploy through a downloadable Go CLI](https://git.subcult.tv/PatrickFanella/pds-ass/issues/27)
- [#28: Deploy on a verified customer-owned PDS endpoint](https://git.subcult.tv/PatrickFanella/pds-ass/issues/28)
- [#29: Migrate an existing AT account with resumable verification](https://git.subcult.tv/PatrickFanella/pds-ass/issues/29)
- [#30: Invite organization members with scoped management roles](https://git.subcult.tv/PatrickFanella/pds-ass/issues/30)

## First work and dependency order

Start [PDS-01 / Gitea #1](https://git.subcult.tv/PatrickFanella/pds-ass/issues/1). Its completion unblocks local PDS qualification, identity/organization onboarding and the staging-node bootstrap. PDS-05 then establishes durable deployment intent. PDS-06 connects those pieces into real provisioning. Work can branch wherever blockers permit it; avoid inventing a serial dependency between independent tests or infrastructure tasks.

M0 proves runnable foundations. M1 proves new-account deployment and BYO handles with invited testing. M2 must pass before hosting customer identities commercially. M3 proves paid service operation and a recoverable exit. M4 is the public-signup gate. M5 features are optional extensions, and none gates the first public launch.

## Release evidence

A phase closes only after its issues satisfy acceptance checks and linked demonstrations. CI passing is necessary where applicable, but does not prove a live PDS, account migration, identity recovery, billing reconciliation or successful restore. Preserve release versions, environment, operation identifiers and secret-free evidence.

The 15-minute managed-domain target must be measured through M3. Report all attempts, failures and retries, distinguish endpoint readiness from first-account completion, and state DNS/identity/downstream-service assumptions. Custom endpoint setup and migration have their own measurements.

## Scope and decisions

Default to one region and prepared shared hosts with isolated PDS containers. Provider, account budget, purchased domains, identity issuer, exact plans/limits, key custody, support hours and retention remain explicit issue-level decisions. No Kubernetes, independent relay network, custom AT implementation, existing-application integration or mandatory npm distribution is required for the first launch.

[Implementation plan](implementation-plan.md) describes architecture, failure behavior and recovery. [Issue index](issues/README.md) provides the complete backlog and blockers.
