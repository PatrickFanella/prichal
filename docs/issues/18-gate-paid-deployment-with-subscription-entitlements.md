# PDS-18: Gate paid deployment with subscription entitlements

<!-- pds-ass-roadmap:18 -->

## What to build

A customer selects a plan, completes hosted checkout and can deploy only after verified subscription entitlement is recorded.

## Acceptance criteria

- [ ] Choose bounded pilot plans, prices, allowances and grace rules as explicit product configuration.
- [ ] Use Stripe test-mode checkout/portal for qualification; process verified signed webhooks idempotently.
- [ ] Reconcile delayed, duplicated and out-of-order billing events before granting or revoking entitlement.
- [ ] The deployment transaction checks entitlement and allowance even if a browser request is forged.
- [ ] Show trial/pilot exceptions as explicit audited grants; payment failure does not immediately delete PDS data.

## Verification

Exercise checkout, webhook replay, failed/late payment, plan change and direct API bypass attempts with test billing; verify one correct entitlement result.

## Blocked by

- PDS-02: Sign in and create an owner-scoped organization
- PDS-05: Submit and resume a durable deployment request

## Boundaries

No live charges in CI, marketplace payouts or ticketing revenue; subscription billing purchases this hosting service.

## Planning reference

Phase: M3 - Paid pilot. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
