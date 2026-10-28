# Managed HTTPS implementation sequence for #7

## Contract and current unit

The owner chose first-level names under `subcult.tv` with no domain purchase. [The namespace policy](managed-namespace.md) reserves generated endpoint and account prefixes and preserves all unrelated first-level names. #6 was accepted and closed after PR #202 merged as `143f109a0d572da272de2cbc9bc93540b44cc3ef` with its required head check passing.

The candidate foundation merged in PR #203 as `789e183ca4ac13be6b61796a83b119d6196e2a72` after its required check passed. It supplies `backend/internal/managednamespace`, `backend/internal/managededge` and the operator-only `managed-edge` command. Candidate generation requires a private versioned inventory, existing placement references for handle bindings, unique generated names/forward ports, a dedicated edge-forward range (35000–35999), exact public-certificate hashes and bounded regular certificate files. The certificate must match the deployment's private ingress identity, be within its validity interval and contain no private PEM material.

The candidate uses exact HTTP host blocks behind the existing Cloudflare public TLS boundary, verified HTTPS upstream transport with a derived certificate mount path, and no wildcard catchall or Authelia redirect on the native PDS protocol surface. Handle candidates serve only GET/HEAD at the exact well-known proof path. The command refuses to replace an existing file or symlink. A manual inventory remains an operator authority input; field validation does not prove its provenance, node placement or bidirectional DID ownership.

PR #204 merged [retained endpoint reservations and authenticated inventory export](managed-routing-inventory.md) as `d6f8314a1b78eef3c1fd1988f416dd2f319ef3bb`. Schema 39 separates public identity from runtime placement and never backfills existing private deployments. The operator can reserve a queued/unplaced native intent, read its current node's public ingress certificate over pinned mTLS, then export the complete qualified inventory. Exports retain a generation and bounded expiry; incomplete, stale or held recovery state is refused. A separately confirmed reservation of the empty staging-02 fixture was qualified without renaming its native runtime. Neither command publishes a route, and handle exports remain empty until verified account bindings exist.

The next unit joins public reservation to the API's durable deployment/event/job commit behind `PDS_STAGING_MANAGED_INTENTS=true`, an additional production-staging opt-in. New public intents retain an empty serving URL and a separate queue, while current workers refuse private fallback before host calls or placement. Existing private requests retain their original identity on replay. [Preparation and rollout limits](managed-routing-inventory.md#atomic-api-intent-preparation) keep this separate from native public activation.

## Remaining units in order

1. Pass retained public intent through worker/client/agent runtime policy while preserving legacy private/test jobs and original-key replay. Replace the current pending gate only when native public identity and readiness checks are supported. Do not derive a replacement name from a new node or silently change a populated PDS. Any controlled fixture conversion still needs a fresh empty-account check, retained predecessor containers/configuration, one-writer fencing and a reversible private fixture procedure.
2. Implement the restricted publisher: inspect current Cloudflare wildcard/edge behavior and host inventory; refuse existing-name/port conflicts; accept only a current authenticated export; stage public certificates and a candidate; validate the complete Caddy configuration; atomically publish only the owned managed import; perform a scoped reload and verify readback. Preserve unrelated routes and a hash-bound rollback candidate. API and ordinary workers receive no DNS, host or Docker administration.
3. Add durable public-readiness checkpoints. DNS/TLS/certificate or upstream failures must remain pending/failed with bounded retry and retained resources, never be accepted from private native health alone. Verify the exact HTTPS origin and expected advertised issuer/service identity, public WebSocket upgrade and unknown-host refusal. Repeating a failed step must retain endpoint, placement and native data.
4. Qualify the complete path on designated staging infrastructure, including a controlled DNS/certificate failure and recovery. Native account enrollment and default handle activation stay in #8; public account creation stays closed. Handle bindings must eventually come from verified account/identity records, not raw customer metadata. Record exact source/candidate/certificate hashes and remaining gates before closing #7.

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

The inventory schema has `version: 1`, `endpoints` and `handles`. Authoritative exports also have a positive `generation` and current `expiresAt`; the renderer refuses expired or unbounded authoritative manifests. Each endpoint contains `deploymentId`, the exact generated `hostname`, a reserved `forwardPort` and `certificateSha256`. Each optional handle contains its `invitationId`, an inventory `deploymentId`, the exact generated `hostname` and an already verified `did:plc` binding. Certificate filenames derive from deployment UUIDs; manifests cannot choose paths, URLs, commands or arbitrary management names.

Generated upstream certificate references use `/etc/caddy/certs/prichal-managed/<deployment UUID>.crt`. A future publisher must verify that these installed files match the input digests and that reserved forwards reach the recorded native instances. This command does not install either file or select an execution window.

## Verification evidence and limits

Focused race tests cover generated names, authority injection/conflicts, unregistered handle placements, certificate validity/digest/symlink/private-material refusal, bounded strict JSON decoding and non-destructive candidate creation. A real candidate using a synthetic public certificate and DID passed validation with the pinned Caddy image in an isolated, network-disabled container. An initial attempt ran as container root and could not read the mode-0600 bind mount; using the operator's numeric UID/GID passed. Existing-output retry was refused with byte-identical content.

This verifies candidate construction and Caddy provisioning only. The subsequent [inventory qualification](managed-routing-inventory.md#native-evidence-and-limits) verifies actual authenticated readback and fenced management snapshot restoration. No public DNS, route, TLS, handle binding or native identity was activated, and no public request was used to claim readiness. #7/#126 remain open. Runtime identity, the restricted publisher, public readiness and failure/retry qualification remain within #7.
