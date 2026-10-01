# PDS-12: Back up and restore a complete customer PDS

<!-- pds-ass-roadmap:12 -->

## What to build

An operator restores a populated customer PDS onto a clean node, retaining its identity, hostname, repositories, media and necessary private preferences.

## Acceptance criteria

- [ ] Back up all PDS-wide and account SQLite databases using a consistent SQLite-aware method, plus protected configuration and referenced blob inventory.
- [ ] Use independent recovery storage with separate access credentials and retention metadata.
- [ ] Restore to a fresh node with fencing that prevents two active writers under the same endpoint.
- [ ] Compare database/repository/blob inventories and authenticate/write again after restoration.
- [ ] Measure achieved data-loss and recovery intervals against the proposed 15-minute/60-minute pilot targets; expose gaps rather than promising them.

## Verification

Populate a disposable PDS, simulate node loss, restore it, compare inventories and prove new writes and downstream events with the original identity.

## Blocked by

- PDS-06: Provision a real PDS through the restricted host agent
- PDS-11: Protect credentials and prove identity-key recovery

## Boundaries

A volume snapshot or account CAR export alone does not complete disaster recovery; retain the old copy until the restored instance is verified.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
