# PDS-13: Restore management state and reconcile recovered jobs

[Gitea issue #13](https://git.subcult.tv/PatrickFanella/pds-ass/issues/13)

<!-- pds-ass-roadmap:13 -->

## What to build

After management database loss, an operator restores organizations and operations and reconnects them to live host inventory without duplicating PDS deployments.

## Acceptance criteria

- [ ] Protect PostgreSQL backups and recovery credentials independently of the active management machine.
- [ ] Include deployment intent, billing references, domains, node identities and audit history in recovery inventory.
- [ ] Pause recovered jobs until reconciliation matches existing host instances by stable IDs.
- [ ] Handle ambiguous/orphaned resources as operator work rather than deleting or duplicating them.
- [ ] Document schema compatibility, recovery order and a rollback checkpoint.

## Verification

Restore a disposable management database while a host runtime exists and a job has an uncertain outcome; verify that resumption allocates no duplicate.

## Blocked by

- #5: Submit and resume a durable deployment request
- #11: Protect credentials and prove identity-key recovery

## Boundaries

Control-plane recovery does not replace PDS data recovery, and billing processor state requires separate reconciliation.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
