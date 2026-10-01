# Local failed-candidate rehearsal

Release qualification must pin an immutable image, retain a verified compatible snapshot, and stage against copied storage before promotion. A failed candidate must never cause an older binary to open data that the candidate may have migrated. Recover compatible storage or keep a fenced original instead.

The local recovery helper now accepts one fixed failure-injection option:

```sh
node scripts/qualify-managed-recovery.mjs .local/owner-enrollment/JOURNEY_UUID bob --candidate-health-fault
```

It creates and authenticates the encrypted snapshot, restores onto fresh volumes and a private network, and starts a pinned-image candidate with a fixed command that exits immediately. The candidate must remain unhealthy. Its writers stop before the untouched original resumes at the same endpoint. Source signed records, media, identity and confirmed account login must remain intact. All original and fresh storage is retained for review.

The actual Bob rehearsal completed on 2026-10-01. It retained 15 files and 5 SQLite databases, verified the encrypted archive, detected the failed candidate and resumed the original with its 3 signed records, referenced media and source identity unchanged. The rehearsal took 3.703 seconds. It does not establish a recovery guarantee for a larger or migrated dataset.

The 2026-10-01 registry check resolved `ghcr.io/bluesky-social/pds:0.4` to the existing pinned digest `sha256:95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336`. No distinct newer release was selected or qualified. This rehearsal does not promote a release or change the managed agent's approved image.

Issue #15 still requires a reviewed distinct-release compatibility matrix, candidate qualification, explicit rollout controls, deployment lifecycle serialization, canary promotion and rollback evidence for actual schema changes. Production rollout remains disabled. See the [upstream PDS distribution](https://github.com/bluesky-social/pds) and [managed recovery](local-managed-recovery.md).
