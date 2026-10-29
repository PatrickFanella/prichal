# Managed staging resumption — October 29, 2026

The original staging-02 managed containers are healthy again and the retained public URL passed fresh protocol checks. This continues [transport recovery](staging-02-transport-renewal.md) under #7; it creates no new tenant, placement, account or route.

## Diagnosis and cold data check

Kvant booted at 02:18:11 UTC, the VM service started at 02:18:23, the guest booted at 02:18:22 and Docker restored container state at 02:18:30. The two managed containers' finish timestamps match that restoration. Their retained restart policy is `no`, so they did not automatically resume. The host/guest boot and explicit policy explain the cold-start mechanism; what initiated the host reboot was not established. An initial journal request had incorrect timestamp quoting; corrected ISO timestamps returned the relevant evidence.

Before resumption, a new same-host cold data/blob archive was extracted into a separate private tree. All nine regular-file hashes matched source and restore, source hashes stayed unchanged, SQLite integrity was ok and account count was zero. Archive SHA256 is `ce15d6e2b7c3768ce394450e849f90d0db032ace421e6c6f2807107d18d9b2a8`. Guest-root custody is `/var/lib/prichal-agent/qualifications/managed-resumption-20261029/`. This verifies a cold data/blob copy, not a booted replacement node or independent/off-host recovery.

## Restricted-agent reconciliation

A private qualification harness used the existing `nodeagent.Client.EnsureManaged` method over registry-pinned mTLS. It locked the fixed deployment/reservation/placement/node, required the converted identity and expected serving URL, active expected node, no unfinished management work, one-slot/three-CPU custody and valid recovery authority around transport. It performed no management-state update or migration, and used no direct Docker start or private-identity fallback.

The existing agent validated retained resources and restarted native container `a7f519a5cc683fbe64602123ee14b336e864a7c121390a1bd8e29b53622ae9c1` and ingress `e86b64f782a4a2f58c91cd65fce343f9b142624f945e7284c76922b5f3eee359`. The first response was running but not yet healthy; subsequent inspection and exact reconciliation replay were healthy. Replay left their start timestamps unchanged. Both private predecessors remained stopped with restart disabled.

Before/after/replay snapshots retained all four IDs, mount bindings, restart policies, native credentials and node key, with one active native writer and zero accounts. Docker changed mount-list ordering; comparison normalized by destination and all fields matched. Native startup can update service data files; this is not a claim of byte-identical running SQLite state. Ingress certificate SHA256 remains `851997f4b23c86fe6bd981e525c5f2bb22ec7bb2e9797a26518284c732624cac`.

Fresh public proof at `https://pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv` passed verified HTTPS, PDS 0.4.5036 health, exact service DID/OAuth endpoints, zero repositories, closed signup, unknown-host 404 and WebSocket acceptance. The existing management checkpoint remains ready, one placement, four old public checks and one retry. No new durable management proof/event was committed; the new protocol observation is separate from that historical checkpoint and does not qualify continuous availability or federation.

## Verification and custody

The managed cold-reconstruction regression in `backend/internal/nodeagent/docker_test.go` resumes both retained containers after reconstructing the agent without recreating containers, credentials, identity receipt or ingress trust. Existing lifecycle-hold checks still prevent reconciliation from undoing an explicit suspension. Focused race checks and full pinned smoke passed (56 Node checks, frontend build, Go vet and all Go race packages). The local verification DB had zero generated smoke/recovery databases and was stopped with its volume retained. Hosted CI remains separate.

Private harness/source, snapshots and protocol receipts are `~/.local/state/prichal-staging-02-bootstrap/managed-resumption/` on Kvant and ignored worktree storage. Harness binary SHA256 is `077e56141cc7aeb01ae3d58e0bf21ec7b27a3fc04295852ce73f173eee69552d`; source SHA256 is `6d8a4f02e211420b55b349170ae17971a29004a6ad61bf7a6d00d3f36d3c32b7`. This is qualification tooling, not a new installed product command. Existing agent/native binaries, credentials, volumes, transport certificate, DNS/Caddy/forwards and capacity were not replaced or reconfigured. No boot-time supervision or restart-policy change was introduced. Node transport expiry remains October 30 at 22:18:46 UTC.

Canonical Machinum evidence is the dated managed-native-resumption report and linked Almaz/Kvant briefings. For pause, fence workers and inspect current ownership before an operator stops only the exact managed native/ingress IDs. Preserve data/keys/receipts and keep private predecessors stopped; never restore the old private identity or snapshots over subsequent writes. Boot-time supervision, fresh API/native allocation and forwarding handoff, verified handle bindings, DNS/provider failure and ongoing renewal/withdrawal remain separate #7 gates; #8 follows #7.
