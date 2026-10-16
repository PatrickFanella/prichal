# Disposable PDS storage qualification

Run the pinned toolchains, then:

```sh
./scripts/pds-local.sh qualify-storage
```

This command creates a fresh official PDS, Caddy ingress and isolated PLC/mail fixture. Each run has a random UUID, labeled Docker resources, separate management-free SQLite/blob volumes and private credentials under ignored `.local/pds-storage/<UUID>/`. Its internal Docker network has no published ports or external attachment. It never uses the retained `pds-ass-runtime` stack, management database, live PDS or public PLC directory.

The harness first creates one disposable `.test` account, uploads a small PNG and writes a record referencing it. Restarting the PDS must retain account login, the record CID and the image bytes. A stopped PDS must return 502 through ingress.

For full-storage qualification, it stops that PDS before starting a second pinned process against the same fresh database. The second process has an **8 MiB tmpfs blob store**. A bounded allocation of at most 9 MiB must produce `ENOSPC`; an authenticated upload must fail. Removing the filler must allow another upload. The original PDS then resumes with its original persistent blob volume, and its record/image must still match. This does not fill the host disk or enforce a production tenant quota.

For corruption qualification, all native writers stop. A separate network-disabled helper saves the fresh fixture's `account.sqlite` and overwrites its first 64 bytes. The PDS must refuse a database-backed administrative check. Restoring that saved database must recover native login, the original record CID and the original image hash. This is deliberate damage to newly generated test data; it is not a supported repair procedure for customer databases.

## Health interpretation

The pinned PDS's `_health` endpoint is a process check. In the October 15 qualification it returned **200 while the full blob store made uploads return 500**. Database corruption prevented startup and yielded 502 through ingress. The report records both process-health and storage/database request results. Treat successful `_health` alone as insufficient evidence that storage is writable or a PDS is ready for account writes.

## Retention and interruption

Every normal success or failure stops all allocated long-lived containers. SIGINT/SIGTERM also attempt to stop every allocated container and write a failed report. Cleanup tries every container even if one stop fails; nonempty `stopFailures` requires operator inspection. The report names retained stopped containers and fresh volumes. The full-storage tmpfs disappears when its container stops; durable original data/blob volumes and the saved pre-corruption database remain.

Credentials, SQLite files and the local PLC/mail fixture are private. Keep them in ignored state. The printed result and private JSON report contain no passwords, tokens, invitation codes or mail bodies. No resources are pruned or deleted by the command. After a hard kill, inspect only resources bearing the exact report UUID label before operating on them; do not restart a retained full-storage or corrupted-database fixture beside another writer.

## Scope of issue #4

The owner confirmed on October 15 that #4 covers disposable qualification; production disk and plan quotas remain in #16/#19. The existing retained local fixture's operator broker keeps its 32-account ceiling, expiring registration lease and single-use invitations. This storage drill creates one account and does **not** qualify that ceiling, concurrent account allocation or production storage enforcement. See [runtime limits and account admission](local-pds-runtime.md) and [native Linux enrollment qualification](local-private-file-readers.md).

The fixture defines CPU/memory/PID limits, a 5 MiB per-upload limit and an isolated 8 MiB filesystem for the full-storage failure case. Durable fixture volumes have no total disk quota. Shared-host storage fairness, commercial allowances and plan enforcement remain separate acceptance work in #16/#19.

## October 15, 2026 evidence

On Kvant's native Linux Docker, Go 1.27.1, Node 24.15.0 and pnpm 12.6.0:

- Three complete runs passed restart persistence, stopped-runtime 502, observed `ENOSPC`, upload failure/recovery, corruption refusal and restored login/record/image comparison. The final command-line run's private report is `.local/pds-storage/810a461d-9f24-4575-a3bd-57214e40656a/report.json`; it retained only stopped fixture containers with no stop failures. Earlier successful reports are `145067e6-fb24-449d-bb1e-e3b83cfeddc8` and `8ef714c0-cd01-4807-8479-2ba9214fadf0` under the same private directory.
- The full `./scripts/dev.sh smoke` gate passed against the separate `prichal-storage-qualification` management volume: 44 Node checks, frontend production build, Go vet and all race-enabled Go packages. The package-manager bootstrap changed the lockfile during verification; that unrelated generated change was removed.
- SIGTERM after the restart checkpoint exited 130, stopped every allocated container and wrote an explicit failed report with no stop failures. Private report: `.local/pds-storage/d5532c45-9bf2-446d-ad45-02028d42413d/report.json`. A prior interruption trial also stopped every fixture, but its surrounding test expected a fixed exit code and failed before checking the report; it is retained separately.
- Initial fixture attempts failed on Caddy's private-file reader identity and the HTTP client's Host handling. The final fixture uses the host agent UID/GID with private Caddy writable paths, and an explicit HTTP Host header through ingress. Those failed runs retain private reports and stopped resources; they are not success evidence.

This is local runtime conformance evidence. Hosted CI, source merge, production deployment, public DNS/TLS, mail delivery, federation, backups and host-loss recovery are separate proof states.
