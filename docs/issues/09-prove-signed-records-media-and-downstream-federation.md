# PDS-09: Prove signed records, media and downstream federation

[Gitea issue #9](https://git.subcult.tv/PatrickFanella/pds-ass/issues/9)

<!-- pds-ass-roadmap:09 -->

## What to build

The customer can authenticate, write and read a signed record and media on the hosted PDS; the service distinguishes local readiness from downstream indexing.

## Acceptance criteria

- [ ] Verify DID/handle/PDS resolution, repository write/read and referenced blob retrieval on a designated identity.
- [ ] Check subscription events through the public WebSocket route.
- [ ] Request or verify relay discovery according to supported upstream behavior and record indexing evidence.
- [ ] Show pending downstream verification distinctly from failure and server readiness.
- [ ] Keep signed write probes bounded and isolated; failed tests do not overwrite customer records.

## Verification

Run a staged account/read/write/blob/WebSocket journey and verify a downstream consumer sees the test record; simulate relay delay and local transport failure.

## Blocked by

- #8: Guide the owner through first-account enrollment

## Boundaries

Hosting does not promise guaranteed Bluesky visibility, immediate indexing, or exemption from downstream policies.

## Planning reference

Phase: M1 - Private deployment alpha. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
