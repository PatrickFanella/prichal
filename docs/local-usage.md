# Native usage observations

The restricted agent exposes a fixed read-only usage operation for an existing healthy, policy-validated PDS. An embedded helper walks only `/pds` and `/blobs`, counts logical file bytes and files, reads the native account count, and measures filesystem headroom. It rejects symlinks, unusual file types, excessive nesting, more than 100,000 entries and unsafe counters. It never returns file names, account identities, database contents or administrator credentials.

Only the authenticated, registry-pinned worker/monitor client can call this operation. The management API reads persisted counters after owner authorization and has no Docker access or node credentials. Host filesystem size and available bytes remain operator-only. Tenant bytes are logical observations, including SQLite databases, companions and temporary files; they are not deduplicated billing units or hard quotas.

The independent monitor atomically records measurements with findings for unavailable usage, fewer than 256 MiB of shared filesystem headroom and more than one native account. Failed measurements preserve previous counters and pressure findings. Older scans cannot replace newer measurements. Owners see unknown, observed, unavailable or stale status with the measurement time. Measurements older than a minute are stale. Total storage limits remain explicitly unconfigured.

```sh
node scripts/qualify-usage-observations.mjs DEPLOYMENT_UUID alice
```

Use `bob` for Bob's local fixture. Run with the monitor watch stopped to avoid overlapping qualification scans. The command validates and stops only the deployment's ingress, confirms unavailable measurements retain the exact previous counters, then starts that same ingress and checks renewed observations. A lock prevents overlapping runs and cleanup preserves all data. It does not stop the PDS, fill the filesystem or enforce a quota.

The 2026-10-01 actual drill passed on Bob's populated tenant. Both owners received positive byte counts and one account; host headroom was absent from owner responses. Controlled ingress failure retained measurements and showed unavailable status; recovery retained the endpoint and container identity.

Scheduled billing aggregation, aggregate disk quotas, disk-pressure admission, off-host alert delivery and realistic resource-pressure qualification remain open for issues #14, #16 and #19.
