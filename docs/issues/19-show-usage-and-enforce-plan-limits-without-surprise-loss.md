# PDS-19: Show usage and enforce plan limits without surprise loss

<!-- pds-ass-roadmap:19 -->

## What to build

An owner sees current account/storage usage and supported limits, and receives a clear upgrade or limit response before creating unsupported extra usage.

## Acceptance criteria

- [ ] Define measurement sources and freshness for blob storage, accounts and deployable instances.
- [ ] Enforce allowance changes at supported runtime/API boundaries, not only in the dashboard.
- [ ] Handle concurrent requests without exceeding instance/account allocation limits.
- [ ] Explain full-storage and downgrade behavior while retaining existing data and allowing export.
- [ ] Reconcile usage after restore and record which figures are estimates.

## Verification

Drive concurrent allocations and controlled storage growth across two plans, then downgrade and restore; verify consistent limits and retained data.

## Blocked by

- PDS-16: Enforce tenant isolation and runtime resource boundaries
- PDS-18: Gate paid deployment with subscription entitlements

## Boundaries

Do not advertise unlimited usage or charge unapproved overages; exact quotas remain product configuration.

## Planning reference

Phase: M3 - Paid pilot. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
