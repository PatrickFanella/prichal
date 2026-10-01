# PDS-15: Upgrade a PDS with staged rollout and safe recovery

<!-- pds-ass-roadmap:15 -->

## What to build

An operator qualifies a new PDS image on a populated staging instance, upgrades one selected tenant and either verifies it or follows a compatible recovery path.

## Acceptance criteria

- [ ] Pin release images and record before/after runtime and schema versions.
- [ ] Create and validate a recovery checkpoint before a potentially incompatible upgrade.
- [ ] Exercise account auth, signed writes, blobs and WebSocket/federation transport before fleet expansion.
- [ ] Prevent unreviewed automatic fleet-wide updates and overlapping maintenance operations.
- [ ] If schemas cannot downgrade safely, restore compatible data rather than run an old image on changed SQLite files.

## Verification

Rehearse one successful and one failed upgrade using populated disposable data; prove the documented rollback or restore preserves identity and content.

## Blocked by

- PDS-09: Prove signed records, media and downstream federation
- PDS-12: Back up and restore a complete customer PDS

## Boundaries

No automatic schema downgrade or blanket reboot/update of every customer PDS.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
