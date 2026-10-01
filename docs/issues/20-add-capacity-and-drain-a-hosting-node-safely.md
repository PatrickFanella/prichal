# PDS-20: Add capacity and drain a hosting node safely

<!-- pds-ass-roadmap:20 -->

## What to build

An operator adds a prepared node, assigns new deployments to it and drains an old node without changing tenant PDS identities or creating concurrent writers.

## Acceptance criteria

- [ ] Track allocatable resources, reservations and placement results transactionally.
- [ ] When capacity is unavailable, show queue/failure state instead of oversubscribing the host.
- [ ] New node enrollment uses the same verified bootstrap workflow.
- [ ] Move a selected PDS using a controlled maintenance/recovery procedure, retain endpoint and fence the original writer.
- [ ] Document capacity thresholds and downtime from actual measurements rather than accounts-per-server guesses.

## Verification

Fill a staging capacity budget, add a node, resume placement and drain one populated tenant to it; verify identity continuity, data inventory and single-writer behavior.

## Blocked by

- PDS-06: Provision a real PDS through the restricted host agent
- PDS-12: Back up and restore a complete customer PDS
- PDS-19: Show usage and enforce plan limits without surprise loss

## Boundaries

One region and measured manual capacity expansion first; no Kubernetes or automatic multi-region failover requirement.

## Planning reference

Phase: M3 - Paid pilot. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
