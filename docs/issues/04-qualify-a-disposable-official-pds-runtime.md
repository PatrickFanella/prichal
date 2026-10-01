# PDS-04: Qualify a disposable official PDS runtime

[Gitea issue #4](https://git.subcult.tv/PatrickFanella/pds-ass/issues/4)

<!-- pds-ass-roadmap:04 -->

## What to build

An operator launches a disposable pinned PDS runtime with separate database and blob storage, then verifies its supported protocol and transport behavior.

## Acceptance criteria

- [ ] Pin the official PDS image by digest and render a reusable runtime manifest.
- [ ] Allocate separate PDS SQLite state, blob namespace and credentials; persist them across a restart.
- [ ] Retain invite codes and repository rate limits; define CPU, memory, disk and account limits.
- [ ] Health and WebSocket checks run through a test ingress; database and storage failures report accurately.
- [ ] Document isolated identity fixtures and a separate designated live-account qualification path.

## Verification

Run a disposable runtime, restart it, test storage and WebSockets, and prove expected data persists; no production identity is created in CI.

## Blocked by

- #1: Run the standalone local stack and CI smoke journey

## Boundaries

Keep the upstream protocol implementation; do not replace SQLite or rewrite PDS functionality.

## Planning reference

Phase: M0 - Runnable foundation. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
