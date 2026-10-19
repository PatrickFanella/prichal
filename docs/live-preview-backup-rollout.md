# Hosted-preview backup rollout — October 19, 2026

Merged PR #162 passed both its PR-head and merged-main CI. Its exact helper and unit bytes were installed on Dozor from commit `90e571a26d88b6bfe9ef27ac349c3277150a4323`, independently of the application release. The database/API/web containers, their start times and the active release link were preserved. Live readiness remained healthy at management schema 35. No admission, native PDS, public route or application-image change was made.

## Installed operations artifacts

| Artifact | SHA-256 |
|---|---|
| `/srv/apps/prichal/operations/preview-backup.py` | `e48debfac81840a452abc579ca415facc8993653c2504f25549dde63be22ed96` |
| `/etc/systemd/system/prichal-preview-backup.service` | `af03cb4e48c5312fa53475fdfb0fba2349eff5aec0535c5fe379ade6676cfce3` |
| `/etc/systemd/system/prichal-preview-backup.timer` | `9aea5745233cf40a2a1b618a0dd7e0be0d84d8fd8b8287bf8efdb6b324475680` |

The systemd service runs as the existing `onnwee` operator with Docker group access and the reviewed restrictions. Its fixed backup directory remains owned by that operator, mode 0700. Archive and receipt files are mode 0600. No previous helper or unit existed to overwrite. The old 127,812-byte `before-decd444-20261003T175305Z.sql` remains retained.

The timer is enabled and active. Its schedule is nightly at 02:40 America/Chicago, plus up to ten minutes of randomized delay, with persistent catch-up. At rollout, the observed next trigger was **October 20, 2026, 02:40:54 CDT** and `LastTriggerUSec` was empty. This is scheduler admission, not proof of a natural nightly execution.

## Manual live-source backup

The first `systemctl start prichal-preview-backup.service` completed with `Result=success` and `ExecMainStatus=0`. It produced:

- Archive: `/srv/apps/prichal/backups/preview-20261019T220712323445Z-38abc310-b80b-4723-8328-e384415a18e2.dump`.
- Size: 171,024 bytes.
- SHA-256: `9dba690c994871d78203633eabfce1dd5057eb48806c1280496901cab6913ae2`, independently read back before enabling the timer.
- Restore: all 60 tables at schema 35; verification completed at `2026-10-19T22:07:15.757120+00:00`.
- Cleanup: the helper's isolated restore container stopped successfully; its tmpfs plaintext was discarded.

The JSON receipt is adjacent to the archive. The fixed helper's retention and interruption rules remain as documented in the [backup runbook](hosted-preview-backups.md). Fourteen Python tests, native synthetic failure/restore checks and full smoke were established in PR #162; this rollout adds actual systemd and live-source evidence.

## Matching-image application restore

A separate fresh clone restored the same live-source archive. Copied management sessions, login attempts, CLI credentials/grants and approval attempts were removed inside that clone before application startup. The exact deployed API image, `prichal-api:decd4442c8dba11b101c7165213fcfaec67b06ed`, ran with local fixture authentication and a synthetic loopback issuer. It received no production OIDC secret or native account credential. No worker started.

The database, issuer, API and HTTP probes shared the database clone's network-disabled namespace. No ports were published. The production database container was never used as the clone's network namespace or connection target. The loopback `/api/readiness` request returned:

```json
{"ready":true,"database":"ready","schemaVersion":35}
```

After startup, the clone remained at schema 35 with zero copied browser/CLI authentication rows. All five qualification containers stopped without failures. The live database/API/web IDs and start times matched their pre-rollout snapshot. The clone's tmpfs database was discarded on stop. Stopped containers and non-secret fixture binaries remain for inspection; they must never be connected to a public route or used as a second writer.

This proves a live-source archive can start the matching API and reach readiness with a synthetic issuer. Real Gitea sign-in, native account operation, worker/job replay, public cutover, off-host custody and host-loss recovery are separate claims.

## Evidence, rollback and remaining gates

Private installation hashes, before/after container snapshots, backup receipt summary and `application-restore.json` are retained on Dozor under `/home/onnwee/.local/state/prichal-preview-backup-rollout/20261019T220649Z-616c2911/`. The application-restore driver and safe readbacks are also retained in this checkout's ignored `.local/`. The canonical Machinum Dozor briefing and dated report were committed as `84796127d045bb67c724e50c31eee8fd031432a8`; this repository owns the maintained helper/runbook. The guarded briefing refresh published Dozor successfully; the other five registered hosts were already current. No website publication is claimed. No private dump or environment file was committed or transferred from Dozor.

Rollback disables only `prichal-preview-backup.timer`, waits for or safely interrupts the owned service, and preserves archives, receipts and manual snapshots. Inspect any exact labeled verifier still running before retrying. Preserve the recorded hashes before removing/replacing only these newly installed operations artifacts, then reload systemd. Application/database rollback is unnecessary because their releases and source data were not changed.

The owner closed #127 on October 19 at 13:39 CDT after merging #162; that closure is preserved. The first natural nightly backup/restore receipt, production owner-admission closure with new-subject denial, monitoring/alert drill and direct Dozor release fetching remain unverified by this rollout. The preview still has one management owner and open admission with zero policy-change audits. No dedicated backup-failure alert was installed during this rollout.
