# PDS-08: Guide the owner through first-account enrollment

[Gitea issue #8](https://git.subcult.tv/PatrickFanella/pds-ass/issues/8)

<!-- pds-ass-roadmap:08 -->

## What to build

After deployment, the owner follows a scoped enrollment flow to create and verify the first AT account on their PDS and log in with a compatible client.

## Acceptance criteria

- [ ] Issue an expiring, tenant-scoped enrollment invitation and prevent reuse.
- [ ] Use the supported PDS/client account flow; management pages do not collect PDS passwords or display administrator credentials.
- [ ] Verification mail uses an approved sender and monitored delivery in staging; tests use a capture transport.
- [ ] Expose clear pending states for verification and failures, and offer bounded owner-authorized invitation recovery.
- [ ] Retain invite-only registration and configured account limits.

## Verification

Complete enrollment and login on a designated staging identity; test wrong tenant, reused invitation, expired invitation and mail failure without creating extra accounts.

## Blocked by

- #2: Sign in and create an owner-scoped organization
- #7: Expose stable managed PDS hostnames with HTTPS

## Boundaries

Fresh accounts only; existing-account migration and delegated organization publishing are separate work.

## Planning reference

Phase: M1 - Private deployment alpha. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
