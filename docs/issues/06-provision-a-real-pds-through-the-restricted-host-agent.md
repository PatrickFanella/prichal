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

## Worker credential preflight — October 28, 2026

The [issue audit](../issue-acceptance-audit-2026-10-28.md#next-issue-6-restricted-provisioning) starts #6 with worker startup validation. Configure `PDS_WORKER_AGENT_CA_FILE`, `PDS_WORKER_AGENT_CERT_FILE` and `PDS_WORKER_AGENT_KEY_FILE` together. Partial settings fail before database connection/migration or job processing; complete settings use the existing certificate/private-key checks. Leaving all three empty supports local simulation only.

Credential preflight alone does not enable a non-local worker. The subsequent private control path below separates provisioning from local-only adapters; public endpoint policy and designated staging interruption/replay qualification remain open. The API receives none of these agent credentials or Docker authority.

## Private staging control path — October 28, 2026

The [private staging provisioning contract](../private-staging-provisioning.md) adds explicit production-mode opt-in, agent-only owner submission and a separate durable worker queue. Existing authentication, entitlement, recovery and agent authority checks remain in force. The worker does not enable other local lifecycle consumers. Live acceptance remains open: Kvant's authorized one-slot staging node already has one retained placement, and the fresh native interruption/replay drill requires designated spare capacity.

## Second-node native evidence — October 28, 2026

The owner authorized dedicated spare capacity and the [fresh native staging drill](../live-staging-02-provisioning.md) passed through the production-mode handler, durable River worker and restricted production-mode staging agent. Injected uncertain completion retained the `configuring` checkpoint; a restarted worker reached ready with one placement/native namespace. Live identity/command/image/capacity/cancellation refusals passed. The management session was synthetic; this does not claim a browser login, OS-kill drill or public hosting. Source integration remains pending before closure.
