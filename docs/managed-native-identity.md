# Managed native identity preparation

This #7 unit carries a retained generated hostname into a fresh native PDS. Public serving remains pending: it installs no DNS, tunnel, public certificate or edge route, leaves the management serving URL empty and cannot mark a managed deployment ready. #7/#126 remain open.

## Opt-ins and custody

The API prepares intent with `PDS_STAGING_MANAGED_INTENTS=true`, as described in [the inventory contract](managed-routing-inventory.md#atomic-api-intent-preparation). The worker and selected node agent separately require `PDS_STAGING_MANAGED_RUNTIME=true`. This runtime opt-in accepts only explicit `PDS_MODE=production` with `PDS_STAGING_PROVISIONING=true`; local, pilot, implicit production and other values fail before database or node startup. Existing transport, loopback listener and staging credential checks remain in force. No running staging service was switched to either opt-in during this unit.

The worker owns the dedicated agent client identity. The API receives no agent credentials, host paths, Docker socket or DNS authority. The node derives `pds-<deployment UUID without hyphens>.subcult.tv` from a canonical UUIDv4. A request cannot supply a hostname, image, environment, path or command. Both client and server retain the existing registry-pinned mTLS transport.

Select a designated enrolled node with native capacity and the runtime opt-in before enabling worker execution. Older or unconfigured agents refuse the new reconciliation route; there is no fallback to private reconciliation or automatic replacement placement. Such native failures use the existing bounded failure path and retain intent/resources. This unit does not add fleet capability discovery.

## Fresh identity and replay

`PUT /v1/managed-instances/<UUID>` accepts only `{}`. Under the agent's existing reconciliation lock, a new identity first requires the absence of UUID-scoped state files, native/ingress/enrollment containers, data/blob volumes and the tenant network. Unavailable inspection also refuses creation. This prevents implicit adoption of retained private credentials, disks or containers.

The agent then writes `<UUID>-managed-identity.json`, a private, versioned receipt containing only the UUID and generated hostname. It uses exclusive atomic link creation, file and directory sync, strict bounded readback and no replacement. The receipt precedes native credentials/resources, so uncertain completion retains one identity. A malformed, symlinked, public or remapped receipt fails closed. Restart requires the same receipt and runtime opt-in; private reconciliation refuses it even if the native container is missing.

Managed native configuration uses the exact public hostname and `did:web` service identity, production protocol mode, real PLC/crawler endpoints and the `.subcult.tv` handle namespace. SMTP and local protocol fixtures are absent. Native invitation enforcement stays enabled; ingress explicitly closes native account creation and OAuth signup. No account/handle binding or email delivery is enabled. First-account/default-handle activation remains #8.

The native tenant uses its isolated outbound bridge, fixed pinned image, CPU/memory/process limits and data/blob volumes. The private ingress keeps the deployment-derived loopback port and retained `<UUID>.localhost` TLS identity. It accepts only that internal host and the generated public host, rejects unknown hosts and validates retained mounts, environment and bindings. Public TLS and host forwarding belong to the future restricted publisher.

With the node runtime opt-in enabled, its HTTP surface permits reconciliation and bounded readback only: ordinary private reconciliation still works for retained private fixtures, and generated reconciliation uses the separate route. All POST mutation routes are closed for this staging mode, including before a managed receipt exists, so a concurrent request cannot obtain enrollment/lifecycle/account/export authority during first creation. Managed readback permits status, usage and the public ingress certificate. The prior local/pilot modes retain their existing handlers.

## Management checkpoints

The worker verifies the generated deployment hostname, queued reservation and empty serving URL before capacity or native execution. It uses the retained placement and calls only `EnsureManaged` for this intent. Recovery authority is checked around native transport; a late hold preserves uncertain host success without checkpointing it or exhausting the job into failure. An allocator-named raw/unreconciled recovery copy cannot execute this path.

Queued intent reserves a native slot and moves to configuring. Successful reconciliation moves to checking with public-readiness guidance. Native health has a two-minute checking window before the existing bounded failure path; failures retain the public reservation, placement and native state. Healthy native resources remain checking and snooze while public readiness is unsupported. The serving URL stays empty throughout this unit. An ordinary private response cannot satisfy managed identity validation.

New queued-intent certificate observations also require the authenticated node to report the exact generated public endpoint. Retained `empty_fixture` reservations remain metadata preparation only; their earlier private readback does not authorize public publication. The restricted publisher must verify native advertised identity and current public readiness rather than treating a certificate alone as proof of public service.

## Verification and limits

Controlled Docker tests cover local, pilot and managed reconciliation, retained resource counts, loopback/network/environment drift, certificate readback and interrupted lifecycle holds. Managed-specific checks cover receipt creation before keys, restart/replay, refusal of retained private files/containers/volumes/networks, remapping and symlink refusal, unsupported mutation routes and hostname injection. Real TLS client/server tests verify generated identity, private-response refusal, status readback and node-key pinning.

PostgreSQL tests use a synthetic agent to qualify uncertain completion/reconstructed worker replay, one retained namespace/placement, native-health timeout, changed reservation/private-response refusal and recovery holds during native transport. They prove management behavior with controlled host responses, not a live staging-node reconciliation.

The optional `TestNativeManagedProtocolEnvironment` passed with the exact pinned PDS image, reporting version `0.4.5036`. It allocated one fresh local container with no network, published ports or persistent volumes, applied the actual generated runtime environment and verified the exact advertised service DID, OAuth issuer/authorization/token endpoints and zero repositories. The container and its temporary storage were removed; unrelated containers were preserved. This verifies that the configuration starts without SMTP. It does not exercise the node agent, managed ingress, public TLS, DNS, WebSockets, outbound PLC reachability or a real account.

To repeat that bounded check with a newly allocated UUID and an already available pinned image:

```sh
cd backend
PDS_TEST_MANAGED_NATIVE_CONTAINER="prichal-managed-env-<fresh UUIDv4>" go test -v -count=1 ./internal/nodeagent -run '^TestNativeManagedProtocolEnvironment$'
```

The test refuses an existing container name and deletes only its newly created, ID/label-verified container. The ordinary smoke gate skips it without the explicit container selection. Private logs and source hashes are under `.local/managed-native/`. Hosted CI is a separate result. The retained Kvant fixtures, their public reservations and their short-lived transport custody were unchanged.

## Pause, rollback and remaining work

Disabling the worker runtime opt-in pauses managed native execution while retaining queued/checking intent and resources. Stop the dedicated worker before reviewing native rollback. Do not delete receipts, regenerate keys, rename populated PDSs or start a second writer from retained disks. A node with managed receipts requires its runtime opt-in to restart. Older node binaries do not support these receipts; do not roll them over managed state as a private fallback. Management schema stays at 39, and the prior preparation worker can retain managed jobs without activating them.

The fresh-only path still refuses implicit conversion. The subsequent [empty staging-02 conversion](managed-staging-conversion.md) records an explicit operator procedure with zero-account evidence, cold native/management restores, one-writer fencing, stopped predecessors and an exercised pre-creation abort. Schema 40 binds this designated conversion to managed worker replay; the live fixture remains checking with an empty serving URL. Separate managed network/configuration names preserve the private predecessors. Read-only production usage now requires the exact generated runtime identity. Public DNS/TLS/issuer/WebSocket readiness and restricted publication remain in #7; account enrollment remains closed.
