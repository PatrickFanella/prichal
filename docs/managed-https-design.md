# Managed HTTPS implementation sequence for #7

## Contract and current unit

The owner chose first-level names under `subcult.tv` with no domain purchase. [The namespace policy](managed-namespace.md) reserves generated endpoint and account prefixes and preserves all unrelated first-level names. #6 was accepted and closed after PR #202 merged as `143f109a0d572da272de2cbc9bc93540b44cc3ef` with its required head check passing.

The current implementation unit supplies `backend/internal/managednamespace`, `backend/internal/managededge` and the operator-only `managed-edge` candidate command. It generates no live inventory and performs no reload. Candidate generation requires a private versioned operator inventory, existing placement references for handle bindings, unique generated names/forward ports, a dedicated edge-forward range (35000–35999), exact public-certificate hashes and bounded regular certificate files. The certificate must match the deployment's private ingress identity, be within its validity interval and contain no private PEM material.

The candidate uses exact HTTP host blocks behind the existing Cloudflare public TLS boundary, verified HTTPS upstream transport with a derived certificate mount path, and no wildcard catchall or Authelia redirect on the native PDS protocol surface. Handle candidates serve only GET/HEAD at the exact well-known proof path. The command refuses to replace an existing file or symlink. Its inventory is an operator authority input; field validation does not prove its provenance, node placement or bidirectional DID ownership.

## Remaining units in order

1. Export an authoritative versioned inventory from retained management endpoint/placement state. Keep it separate from customer JSON and require explicit delivery to the restricted edge operator. Bind public certificate readback to the recorded deployment/node identity. Handle bindings must come from verified account/identity records, not raw customer metadata.
2. Allocate the public endpoint in durable intent before runtime activation. Pass its retained identity through worker/client/agent policy while preserving legacy private/test jobs and original-key replay. Do not derive a replacement name from a new node or silently change a populated PDS. The existing second-node fixture is empty, but any controlled conversion still needs an explicit empty-account check, retained predecessor containers/configuration, one-writer fencing and a reversible private fixture procedure.
3. Implement the restricted publisher: inspect current Cloudflare wildcard/edge behavior and host inventory; refuse existing-name/port conflicts; stage public certificates and a candidate; validate the complete Caddy configuration; atomically publish only the owned managed import; perform a scoped reload and verify readback. Preserve unrelated routes and a hash-bound rollback candidate. API and ordinary workers receive no DNS, host or Docker administration.
4. Add durable public-readiness checkpoints. DNS/TLS/certificate or upstream failures must remain pending/failed with bounded retry and retained resources, never be accepted from private native health alone. Verify the exact HTTPS origin and expected advertised issuer/service identity, public WebSocket upgrade and unknown-host refusal. Repeating a failed step must retain endpoint, placement and native data.
5. Qualify the complete path on designated staging infrastructure, including a controlled DNS/certificate failure and recovery. Native account enrollment and default handle activation stay in #8; public account creation stays closed. Record exact source/candidate/certificate hashes and remaining gates before closing #7.

## Generate a candidate

Build with the pinned Go toolchain:

```sh
cd backend
go build -o ../.local/managed-edge/managed-edge ./cmd/managed-edge
```

Run with a private operator inventory and public certificates copied through the approved custody path:

```sh
.local/managed-edge/managed-edge   --inventory .local/managed-edge/inventory.json   --certificates /absolute/private/certificate-directory   --out .local/managed-edge/new-candidate.Caddyfile
```

The inventory schema has `version: 1`, `endpoints` and `handles`. Each endpoint contains `deploymentId`, the exact generated `hostname`, a reserved `forwardPort` and `certificateSha256`. Each optional handle contains its `invitationId`, an inventory `deploymentId`, the exact generated `hostname` and an already verified `did:plc` binding. Certificate filenames derive from deployment UUIDs; manifests cannot choose paths, URLs, commands or arbitrary management names.

Generated upstream certificate references use `/etc/caddy/certs/prichal-managed/<deployment UUID>.crt`. A future publisher must verify that these installed files match the input digests and that reserved forwards reach the recorded native instances. This command does not install either file or select an execution window.

## Verification evidence and limits

Focused race tests cover generated names, authority injection/conflicts, unregistered handle placements, certificate validity/digest/symlink/private-material refusal, bounded strict JSON decoding and non-destructive candidate creation. A real candidate using a synthetic public certificate and DID passed validation with the pinned Caddy image in an isolated, network-disabled container. An initial attempt ran as container root and could not read the mode-0600 bind mount; using the operator's numeric UID/GID passed. Existing-output retry was refused with byte-identical content.

This verifies candidate construction and Caddy provisioning only. No public DNS, route, TLS, handle binding, native identity or service was changed, and no public request was used to claim readiness. #7/#126 remain open. Public readiness, the authoritative inventory/publisher and real failure/retry qualification are the next work within #7.
