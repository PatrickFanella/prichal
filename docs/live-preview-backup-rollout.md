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

## First natural timer run — October 20, 2026

The enabled timer fired naturally at **02:43:15 CDT** (`07:43:15 UTC`). Readback before the trigger showed an empty `LastTriggerUSec`; no manual start was issued for this observation. The service finished at **02:43:21 CDT** with `Result=success` and `ExecMainStatus=0`.

- Archive: `/srv/apps/prichal/backups/preview-20261020T074316349124Z-ee6a8e81-aea8-4a7c-946e-b63526772c65.dump`.
- Independently checked size: **171,024 bytes**.
- Independently checked SHA-256: `116d35d52e2191697803e98b03ca1d80d4ab0d04972c372cda2818b67e820a54`.
- Receipt verification: `2026-10-20T07:43:21.021109+00:00`, **60 tables**, management schema **35**.
- Restore container: `prichal-preview-restore-1fd688d6-0eea-4c23-93a8-5ee52d6bb767`, independently confirmed `exited`, network `none`, with only the configured tmpfs targets.

At `07:44:22 UTC`, the timer remained enabled/active with its next trigger October 21 at **02:49:57 CDT**. Archive and receipt modes were 0600 and the backup directory was 0700. Two generated archives and the older manual SQL snapshot remained retained. Installed helper/service/timer hashes still matched the rollout table above. The live source container matched the receipt; database/API/web containers were running with start times before this natural run. The actual LAN-bound readiness endpoint returned `ready=true`, `database=ready`, schema 35. An initial loopback-address probe was refused; the documented LAN address was then used successfully.

The safe readback is retained in this checkout at `.local/issue-audit/preview-first-natural-run.json`, alongside the archive/receipt on Dozor. The canonical Machinum Dozor briefing and dated rollout report record the same evidence. This proves one natural scheduled backup and isolated database restore; it does not prove repeated retention/pruning, backup-failure alert delivery, off-host custody, job replay or a new matching-image application restore. The October 19 application-readiness qualification below remains separate. No deployment, admission change or service restart was performed for this observation.

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

The owner closed #127 on October 19 at 13:39 CDT after merging #162; that closure is preserved. The first natural nightly backup/restore receipt is verified above. Production owner-admission closure with new-subject denial, monitoring/alert drill and direct Dozor release fetching remain unverified by this rollout. The preview still has one management owner and open admission with zero policy-change audits. No dedicated backup-failure alert was installed during this rollout.
