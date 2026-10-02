# PDS-18: Gate paid deployment with subscription entitlements

[Gitea issue #18](https://git.subcult.tv/PatrickFanella/pds-ass/issues/18)

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

- #2: Sign in and create an owner-scoped organization
- #5: Submit and resume a durable deployment request

## Boundaries

No live charges in CI, marketplace payouts or ticketing revenue; subscription billing purchases this hosting service.

The [local Checkout/Portal adapter](../local-billing-sessions.md) now has SDK transport and failure qualification. It exposes no owner route and grants no entitlement. Durable customer/request mapping, browser review, completion-event reconciliation, actual sandbox resources and commercial policy remain open.

[Checkout preparation](../local-checkout-preparation.md) now records original/current owner scope and a stable configured fixture plan with atomic review audit and read-only key recovery. It dispatches no payment and grants no entitlement. Schema-31 upgrade and source recovery passed; customer mapping, dispatch and browser routes remain open.

[Durable fixture dispatch](../local-checkout-dispatch.md) now checkpoints first-class customer/session IDs with authority/credential binding and a bounded original-key retry window. SQL/SDK failure and schema-32 recovery checks passed. No source checkout was dispatched and no entitlement was granted. Owner routes, hosted-session recovery, populated restore, browser review and required completion reconciliation remain open.

## Planning reference

Phase: M3 - Paid pilot. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).

[Read-only session recovery](../local-checkout-session-recovery.md) now verifies the original confirmed fixture checkout without creating replacement resources. It returns only transient navigation for open sessions and never grants payment entitlement. Owner HTTP/browser and completion reconciliation remain open.

[Owner checkout routes](../local-checkout-owner-routes.md) now expose local preparation, explicit original-key dispatch and read-only session recovery under real browser authentication/CSRF. They remain separately disabled by default. Browser review, actual fixture journey and completion/entitlement reconciliation remain open.

[Scoped browser checkout review](../local-checkout-workspace.md) now retains the original fixture key/Price, separates preparation and dispatch reviews, and recovers only transient confirmed navigation. Intercepted browser verification and real disabled reload checks passed; other responsive widths, actual authority journey, populated restore and completion/entitlement reconciliation remain open.
