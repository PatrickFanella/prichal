# PDS-06: Provision a real PDS through the restricted host agent

[Gitea issue #6](https://git.subcult.tv/PatrickFanella/pds-ass/issues/6)

<!-- pds-ass-roadmap:06 -->

## What to build

An invited owner requests a deployment and the worker starts one PDS on a registered staging node, preserving the instance after worker retries.

## Acceptance criteria

- [ ] Placement reserves measured host capacity and an instance-specific persistent volume/storage namespace.
- [ ] Worker and agent use authenticated private communication and an allowlisted lifecycle contract.
- [ ] Neither public API nor customer runtime receives host Docker administration, provider credentials or arbitrary execution capability.
- [ ] A crash after container creation reconciles by stable instance ID before retrying.
- [ ] Exhausted capacity, agent timeout and uncertain completion leave diagnosable state without deleting user data.

## Verification

Deploy to staging, interrupt the worker after host success, retry and verify exactly one runtime/volume; reject unregistered agents and arbitrary image/path requests.

## Blocked by

- #3: Bootstrap and register a staging hosting node
- #4: Qualify a disposable official PDS runtime
- #5: Submit and resume a durable deployment request

## Boundaries

Deploy only pinned service-managed PDS runtimes; no customer code hosting or per-request VM installation.

## Planning reference

Phase: M1 - Private deployment alpha. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
