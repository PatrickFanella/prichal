# Managed HTTPS implementation sequence for #7

## Contract and current unit

The owner chose first-level names under `subcult.tv` with no domain purchase. [The namespace policy](managed-namespace.md) reserves generated endpoint and account prefixes and preserves all unrelated first-level names. #6 was accepted and closed after PR #202 merged as `143f109a0d572da272de2cbc9bc93540b44cc3ef` with its required head check passing.

The candidate foundation merged in PR #203 as `789e183ca4ac13be6b61796a83b119d6196e2a72` after its required check passed. It supplies `backend/internal/managednamespace`, `backend/internal/managededge` and the operator-only `managed-edge` command. Candidate generation requires a private versioned inventory, existing placement references for handle bindings, unique generated names/forward ports, a dedicated edge-forward range (35000–35999), exact public-certificate hashes and bounded regular certificate files. The certificate must match the deployment's private ingress identity, be within its validity interval and contain no private PEM material.

The candidate uses exact HTTP host blocks behind the existing Cloudflare public TLS boundary, verified HTTPS upstream transport with a derived certificate mount path, and no wildcard catchall or Authelia redirect on the native PDS protocol surface. Handle candidates serve only GET/HEAD at the exact well-known proof path. The command refuses to replace an existing file or symlink. A manual inventory remains an operator authority input; field validation does not prove its provenance, node placement or bidirectional DID ownership.

PR #204 merged [retained endpoint reservations and authenticated inventory export](managed-routing-inventory.md) as `d6f8314a1b78eef3c1fd1988f416dd2f319ef3bb`. Schema 39 separates public identity from runtime placement and never backfills existing private deployments. The operator can reserve a queued/unplaced native intent, read its current node's public ingress certificate over pinned mTLS, then export the complete qualified inventory. Exports retain a generation and bounded expiry; incomplete, stale or held recovery state is refused. A separately confirmed reservation of the empty staging-02 fixture was qualified without renaming its native runtime. Neither command publishes a route, and handle exports remain empty until verified account bindings exist.

PR #205 merged atomic API deployment/reservation/event/job commit as `4e4e055385da4f286e870f59a89605a3cb1d9d85` after its required check passed. `PDS_STAGING_MANAGED_INTENTS=true` is an additional production-staging API opt-in. New public intents retain an empty serving URL and a separate queue. Existing private requests retain their original identity on replay. [Preparation and rollout limits](managed-routing-inventory.md#atomic-api-intent-preparation) keep this separate from native public activation.

PR #206 merged the [native identity unit](managed-native-identity.md) as `ac80348adb8a89f5baa7cbcca82506dd3fee6760` after its required check passed. A separate worker/node runtime opt-in permits generated-only reconciliation, writes a retained identity receipt before fresh resources and keeps registration closed. Native health cannot mark public intent ready. The subsequent [designated empty staging conversion](managed-staging-conversion.md) qualified cold native/management restores, one-writer fencing, private protocol behavior and real managed-worker replay. The live fixture remains checking with an empty serving URL; public routing is unfinished.

PR #207 merged the designated conversion as `939dfb2624ca3ad9e607aeb4b054e2bd9f588693`, with required head and merged-main checks passing. The subsequent [restricted staging publisher](managed-staging-publication.md) qualified initial exact-host publication, public TLS/native/OAuth/WebSocket observations, guarded route withdrawal and fresh-export republication. Its main Caddyfile and unrelated inputs stayed unchanged. This publisher supports only the designated fixture; management remains checking with no serving URL.

PR #208 merged that initial publisher as `310973f91706da548d4d6bfc5d809590452836bc`, with required head and merged-main checks passing. The [durable public readiness unit](managed-public-readiness.md) adds a separate worker opt-in, bounded check receipts and same-identity operator retry. The live fixture reached ready with its public URL after a worker-local trust failure, restart, retry and reconstruction following uncertain public success. General publisher updates and DNS/provider custody/failure gates remain open.

## Remaining units in order

1. Retain the qualified staging-02 identity and private conversion evidence. Full recovery after managed writes and off-host backup remain separate gates. Do not derive a replacement name from a new node or silently change a populated PDS. Keep public intent pending until the remaining readiness work is supported.
2. Extend the qualified initial staging publisher to supported authoritative inventory updates and additional placements, preserving exact-host/port conflict refusal, current authenticated export custody, immutable public trust, full Caddy validation, ETag-fenced reload/readback and hash-bound rollback. Its present fixed-fixture scope deliberately refuses other endpoints, handles and existing-route replacement. API and ordinary workers receive no DNS, host or Docker administration.
3. Retain the qualified initial public-readiness checkpoints and same-identity retry. Extend supported provider/DNS failure qualification and ongoing renewal/withdrawal policy without accepting private native health alone or losing endpoint, placement or data. Current live failure evidence is a process-local TLS trust failure; it does not claim a DNS/provider outage.
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

This initial check verifies candidate construction and Caddy provisioning only. Subsequent [inventory qualification](managed-routing-inventory.md#native-evidence-and-limits), [conversion evidence](managed-staging-conversion.md) and [public staging publication](managed-staging-publication.md) record later native and public observations. The fixed-fixture route now exists, but no handle binding or durable public readiness was established. #7/#126 remain open for general publication/update authority, public readiness and failure/retry qualification.
