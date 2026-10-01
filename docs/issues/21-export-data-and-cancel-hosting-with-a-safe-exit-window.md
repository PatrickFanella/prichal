# PDS-21: Export data and cancel hosting with a safe exit window

<!-- pds-ass-roadmap:21 -->

## What to build

An owner can export the supported account data and cancel hosting while retaining a clear opportunity to migrate or recover before scheduled deletion.

## Acceptance criteria

- [ ] Provide scoped repository/blob/preference exports and document state that lives outside the PDS.
- [ ] Require owner authorization and expire export access; never include provider secrets or other accounts' data.
- [ ] Cancellation records an explicit service end and configurable retention/deletion schedule.
- [ ] Allow recovery or cancellation reversal within the supported window; irreversible deletion requires the documented owner confirmation path.
- [ ] Provide an exit rehearsal with migration instructions and evidence of completed export.

## Verification

Export populated test data, cancel in test billing, recover during the retention window and rehearse confirmed deletion on disposable data; compare the export inventory.

## Blocked by

- PDS-12: Back up and restore a complete customer PDS
- PDS-17: Suspend and resume service through audited operator actions
- PDS-18: Gate paid deployment with subscription entitlements

## Boundaries

Do not confuse cancelling payment with deleting identity or all replicated public content; migration automation is a later extension.

## Planning reference

Phase: M3 - Paid pilot. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
