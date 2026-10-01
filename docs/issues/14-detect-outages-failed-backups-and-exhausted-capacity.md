# PDS-14: Detect outages, failed backups and exhausted capacity

<!-- pds-ass-roadmap:14 -->

## What to build

An operator receives actionable alerts for PDS availability, certificate/storage problems, job backlog and stale recovery evidence.

## Acceptance criteria

- [ ] Collect bounded tenant/node metrics and structured logs without account passwords, keys or private records.
- [ ] Use an external HTTPS/WebSocket probe that can detect failure of the hosting network itself.
- [ ] Alert on disk/storage thresholds, exhausted placement capacity, stale backups, restore failure and queued/failed operations.
- [ ] Expose customer-readable service status and last backup/recovery-test information.
- [ ] Write incident runbooks with ownership, diagnostic steps and alert deduplication.

## Verification

Inject endpoint, disk, queue and backup failures and verify the intended alert and operator runbook; verify no sensitive payload enters monitoring.

## Blocked by

- PDS-07: Expose stable managed PDS hostnames with HTTPS
- PDS-12: Back up and restore a complete customer PDS
- PDS-13: Restore management state and reconcile recovered jobs

## Boundaries

Monitoring must work when the application is down; no uptime/SLA claim is inferred from installing dashboards.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
