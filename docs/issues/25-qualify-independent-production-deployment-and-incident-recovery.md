# PDS-25: Qualify independent production deployment and incident recovery

[Gitea issue #25](https://git.subcult.tv/PatrickFanella/pds-ass/issues/25)

<!-- pds-ass-roadmap:25 -->

## What to build

An operator can deploy the management service and prepared fleet to the selected production account, recover them and operate while development/homelab systems are offline.

## Acceptance criteria

- [ ] Choose production provider/region/domains, identity issuer, mail sender and secret/backup custody with explicit budget and access ownership.
- [ ] Use separate staging and production credentials, infrastructure state, billing mode and host inventories.
- [ ] Release pinned API/web/worker/agent images through CI with migration and health gates.
- [ ] Run production-design recovery and incident drills using designated disposable tenants before opening signup.
- [ ] Document on-call/support hours, rollback steps and dependency failures; commercial serving/recovery does not depend on the workstation.

## Verification

Recreate the production-design environment from versioned configuration, verify auth/billing/mail routing, then rehearse management and PDS recovery with development access unavailable.

## Blocked by

- #3: Bootstrap and register a staging hosting node
- #11: Protect credentials and prove identity-key recovery
- #13: Restore management state and reconcile recovered jobs
- #14: Detect outages, failed backups and exhausted capacity
- #15: Upgrade a PDS with staged rollout and safe recovery
- #18: Gate paid deployment with subscription entitlements

## Boundaries

This issue is a qualification gate; actual paid infrastructure creation or public changes require the selected account and authorized execution window.

## Planning reference

Phase: M4 - Public launch. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
