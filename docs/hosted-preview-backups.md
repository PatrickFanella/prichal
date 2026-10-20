# Private preview database backups

This delivers the backup tooling portion of #127. The reviewed helper and systemd units from merged PR #162 were installed on Dozor on October 19. A live-source backup, isolated database restore and matching-image application-readiness restore passed. The nightly timer is enabled; its first natural execution remains pending. The preview application remains on schema 35 with its existing API/web images and open owner admission. [Live rollout evidence](live-preview-backup-rollout.md) records hashes, the private archive, schedule and remaining gates.

`deploy/production/preview-backup.py` provides three commands:

```sh
python3 deploy/production/preview-backup.py qualify
python3 /srv/apps/prichal/operations/preview-backup.py backup
python3 /srv/apps/prichal/operations/preview-backup.py verify preview-<timestamp>-<UUID>.dump
```

`verify` returns a fresh restore result without rewriting receipts or pruning. Qualification runs only in a complete repository checkout and writes synthetic artifacts under ignored `.local/preview-backups/<UUID>/`. Production backup/verify use only `/srv/apps/prichal/backups`. There are no caller-supplied database URLs, source container names or output directories. Python 3.11+ and Linux Docker are required; Dozor currently has Python 3.14.6. The pinned PostgreSQL image must already be present: the verifier uses `--pull=never`.

## Backup and restore boundaries

The production source is exactly `prichal-postgres-1`, with the `prichal` Compose project, `postgres` service, fixed image digest, sole `prichal_management-data` volume and no published database port. The helper resolves and uses that container ID, then dumps only the `prichal` database. It refuses other identities or a missing management schema. It neither reads production environment files nor changes the source database, admission policy, application containers or PDS accounts. Docker always uses the local Unix socket with an empty private client configuration; the caller's remote context and registry credentials are ignored. This host-side tool has Docker authority; it is not imported by or exposed through the management API.

Each archive is a consistent PostgreSQL custom-format dump. Source SQL has a 30-second statement timeout; `pg_dump` statements have a two-minute limit and the client has a three-minute process timeout. An inherited file-size limit caps each dump at 256 MiB. Files are private mode 0600 in an owned 0700 directory; symlinks, hard links, FIFOs and unsafe locks are refused. A non-blocking exclusive lock excludes overlapping backup/verification runs. Archive and JSON receipt publication uses temporary files and fsync; receipts contain a SHA-256 checksum, size, source container ID and schema version, without owner identities, tokens or database rows. These local files contain private management authentication data and must remain on protected storage.

Before recording success or pruning, the helper checks the archive checksum and restores the entire dump into a fresh PostgreSQL container. The clone has network `none`, no published ports or persistent volumes, a read-only root, dropped capabilities and explicit memory/CPU/PID/tmpfs limits. The local-only database role uses trust authentication inside that unreachable container; no production password is copied to its environment. The restored schema must match the captured version. Source database privileges and roles beyond the application's `prichal` role are not qualified: dump/restore explicitly omit ownership and ACL statements. No API, worker, OIDC client or scheduler starts against the copy.

The clone is stopped before verification is recorded. Its tmpfs plaintext disappears on stop; the stopped container remains available for non-secret state inspection. Restore, allocation, timeout, SIGINT/SIGTERM and stop failures remain failures. Private PostgreSQL stderr is suppressed to prevent row contents appearing in journals. A failed or interrupted run does not get a success receipt and does not prune older backups. SIGKILL, host loss and a Docker daemon outage cannot guarantee immediate container cleanup; inspect the helper's exact qualification label/name before restarting after those failures.

## Retention and failure handling

A successful verified run keeps the newest 14 generated archive/receipt pairs. Every candidate pair is validated before deletion. Existing `before-*.sql` snapshots and other unrelated files are preserved. A failed verification can retain one additional pair: at 15 archives, new allocation stops until an operator reviews the failure. An interrupted `.partial-*` or `.receipt-*` also blocks new allocation and is preserved for review. This bounds generated completed dumps to 3.75 GiB plus one in-progress file/receipt and excludes historical/manual snapshots. It is not a filesystem quota. Receipts, lock and partial files are small except for a dump partial, which has the same 256 MiB cap.

On failure, inspect the systemd result and private artifacts. Verify whether the exact labeled restore container is still running. Preserve the last verified backup; examine any failed pair or orphan before moving it to protected incident custody or removing it. The helper does not broad-prune files, containers or volumes to make a retry succeed. A checksum failure occurs before any restore container is allocated.

The archive covers management PostgreSQL only. Native PDS SQLite databases, blobs, identity keys, OIDC client configuration, external routing, off-host custody and host-loss recovery have separate owners and procedures. A matching restored schema and successful `pg_restore` do not establish application readiness, copied-session revocation, job reconciliation or a browser sign-in.

## Installation procedure on Dozor

Install from an exact reviewed checkout without updating the preview API, web images or management schema. Keep a copy and hash of any prior helper/units for rollback. `/srv/apps/prichal/operations/preview-backup.py` is a stable operations artifact, independent of the active application release. The units run as the existing `onnwee` operator with Docker group access and private permissions.

```sh
PRICHAL_BACKUP_SOURCE=/absolute/path/to/reviewed/prichal
sudo install -d -o onnwee -g onnwee -m 0750 /srv/apps/prichal/operations
sudo install -d -o onnwee -g onnwee -m 0700 /srv/apps/prichal/backups
sudo install -o root -g root -m 0755 "$PRICHAL_BACKUP_SOURCE/deploy/production/preview-backup.py" /srv/apps/prichal/operations/preview-backup.py
sudo install -o root -g root -m 0644 "$PRICHAL_BACKUP_SOURCE/deploy/production/prichal-preview-backup.service" /etc/systemd/system/
sudo install -o root -g root -m 0644 "$PRICHAL_BACKUP_SOURCE/deploy/production/prichal-preview-backup.timer" /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl start prichal-preview-backup.service
sudo systemctl status prichal-preview-backup.service --no-pager
```

Review the resulting private receipt and stopped clone before enabling the timer:

```sh
sudo systemctl enable --now prichal-preview-backup.timer
systemctl list-timers prichal-preview-backup.timer --no-pager
```

The timer runs nightly at 02:40 in the host's timezone, with up to ten minutes of randomized delay and catch-up after downtime. The service has a ten-minute startup limit, a 30-second stop window and restricted writable paths. The October 19 installation and manual live-source/application restore are recorded in the rollout evidence. The first natural nightly run passed on October 20 at 02:43 CDT, including an independently checked archive checksum and stopped 60-table/schema-35 restore; see the rollout evidence. Record that receipt, later failures and rollback evidence in this repository and the canonical Machinum Dozor host note; do not edit generated `~/.homelab/HOMELAB.md`.

Rollback disables only `prichal-preview-backup.timer`, waits for or safely interrupts its owned running service, then restores the reviewed prior helper/units if present and reloads systemd. Retain all backup files and stopped verification containers. No application/database rollback is needed because the helper does not modify their release or source data.

## Qualification evidence

Fourteen deterministic Python tests passed, covering exact source/cleanup scope, private files and locking, caller Docker-context refusal, tampering before allocation, restore/allocation failure cleanup, failed-success/pruning prevention, retained evidence, and successful/failed retention bounds. `systemd-analyze verify` accepted both unit files. The Gitea workflow runs the Python checks alongside the existing smoke gate.

Native full-schema qualification `15ab72c7-9f2a-4374-ab38-04e1ad85264f` built all 36 management migrations in a fresh source, seeded a synthetic owner/organization plus a row/identity-sequence canary, then dumped and restored 57 tables. The restored canary row, next sequence value and owner/organization relationship passed. Archive/receipt evidence remains in `.local/preview-backups/15ab72c7-9f2a-4374-ab38-04e1ad85264f/`. Both fixtures stopped; no source production data was copied. Earlier narrower and full-schema runs remain separate.

A separate native SIGTERM during restore exited 1 with no success output and stopped both allocated source and restore containers. Its safe report is `.local/preview-backup-interruption.json`. Source application tests and public readiness remain separate from this synthetic restore. Those implementation-qualification runs preceded deployment. The later rollout installed the timer, verified a live-source/application-readiness restore and updated the canonical host note. The first natural nightly execution is now verified in the rollout evidence. Production owner-admission closure, monitoring and direct release transport remain unverified by this rollout; the owner subsequently closed #127.
