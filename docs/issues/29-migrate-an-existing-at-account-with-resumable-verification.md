# PDS-29: Migrate an existing AT account with resumable verification

[Gitea issue #29](https://git.subcult.tv/PatrickFanella/pds-ass/issues/29)

<!-- pds-ass-roadmap:29 -->

## What to build

A customer moves an existing AT account onto or away from the service using explicit account authorization and a resumable inventory-based migration.

## Acceptance criteria

- [ ] Preflight old-host access, account authority, repository size, blobs, preferences and recovery options.
- [ ] Transfer and verify data before an explicitly authorized identity switch.
- [ ] Resume partial imports without duplicate/lost records and display unsupported old-host conditions.
- [ ] Check post-switch auth, record/blob inventory, signing and downstream identity resolution.
- [ ] Document failure/rollback boundaries and confirm state held outside the PDS; never claim a universal automatic rollback after identity mutation.

## Verification

Rehearse inbound and outbound migration between designated populated test hosts; interrupt transfers and inject missing blobs/stale identity resolution.

## Blocked by

- #9: Prove signed records, media and downstream federation
- #12: Back up and restore a complete customer PDS
- #21: Export data and cancel hosting with a safe exit window

## Boundaries

Real-account migration requires its owner's authorization; complex/adversarial recovery and guaranteed completion times are separate qualifications.

## Planning reference

Phase: M5 - Additional access and portability. Priority: P2.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).

## Local implementation evidence

[Retained-export inventory preflight](../local-migration-preflight.md) authenticates existing owned unexpired archives and reports bounded historical inventory without target access or identity mutation. Both archive versions passed independent offline signature/media checks on the four retained local artifacts. Current source authority, designated migration targets, resumable import and identity switching remain unqualified; acceptance criteria remain open.
