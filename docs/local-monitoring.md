# Local operational monitoring

Run `./scripts/monitor-local.sh once` for one scan or `./scripts/monitor-local.sh watch` for a foreground scan every 30 seconds. The monitor connects directly to the separate local management database and the authenticated restricted node agent. The public management API may be unavailable while monitoring still works. Keep its private worker certificate and logs inside `.local/`.

The monitor records host availability, retained capacity, placed-runtime health, deployments stuck for more than two minutes, failed deployments and recovery holds. Findings contain fixed kinds, resource IDs and timestamps. Raw Docker errors, account passwords, repository records and keys are excluded. Successful scans update findings atomically. Repeated failures share a subject and kind, recovery records a resolution timestamp, and failed scans retain prior findings. Owner-visible status is authorized through the deployment's organization and becomes stale after one minute without a current runtime observation.

Scheduled backups are not configured, so monitoring always reports `backup_unconfigured`. Local encrypted restore reports do not establish backup freshness. External network probes, disk metrics, certificate-expiry alerts, independent monitoring storage, scheduler installation and alert delivery remain open. This monitor does not establish an uptime or response-time commitment.

## Local incident procedure

The local operator owns these qualification alerts. A real on-call owner and delivery channel must be selected before live qualification.

- `host_unavailable`: verify the local agent listener and certificate validity, then inspect its private logs. Do not bypass its identity check or recreate credentials to silence an alert.
- `runtime_unhealthy`: inspect the fixed PDS and ingress containers. Confirm the stable TLS endpoint with its retained fixture certificate. Preserve volumes and identity files. Use the recovery runbook if retained resources conflict.
- `capacity_exhausted`: inspect retained namespaces and placements. Failed deployments do not authorize deletion of older fixtures. Drain or add verified capacity before retrying.
- `job_backlog` or `deployment_failed`: compare management checkpoints with host inventory. A recovered database must remain held until reconciliation. Retry the same stable operation only after identifying the cause.
- `recovery_held`: follow `docs/local-recovery.md`. Missing and untracked namespaces require explicit investigation. Do not directly change the ledger to release the hold.
- `backup_unconfigured`: configure and qualify independent storage, key custody, backup scheduling and restores before customer launch.
- A failed monitor or stale service observation requires inspection of the monitor, agent and management database. An old healthy observation is not evidence of current health.

## Qualification

With the local management API stopped, run `node scripts/qualify-monitor.mjs` with `.local/local.env` loaded. The script stops only the single placed qualification ingress, scans twice, checks one active outage finding, restarts that same container in a `finally` block and confirms recovery. It preserves PDS data and container identity. Restart the management API after the drill. Private evidence stays in `.local/monitor/`.
