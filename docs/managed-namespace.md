# Managed namespace decision — October 28, 2026

The owner selected first-level names under `subcult.tv`, with no domain purchase, for #126. Reuse the existing domain and Cloudflare DNS/edge service. Public NS lookup on October 28 returned `dahlia.ns.cloudflare.com` and `hasslo.ns.cloudflare.com`; this confirms delegation, not registrar or account access. The existing domain custodian retains control; no registrar transfer, certificate purchase or zone-wide DNS permission is implied.

The #7 design must keep managed endpoint and default-handle labels one level below `subcult.tv`, preserve management/OAuth and existing service/pilot labels, and reserve separate collision-safe prefixes for endpoints and accounts. Exact prefix allocation, reserved names, handle issuance/abuse limits and inventory-only routing must be specified and verified before name issuance. Nested `*.pds.subcult.tv` is outside this decision.

This records the chosen namespace. It changes no DNS, certificate, route, PDS identity or signup policy. #126 remains open for its remaining design/ownership evidence; #7 remains gated by #6 and actual DNS/TLS/WebSocket/failure qualification. The hosted #126 and #7 issue bodies record the same decision.

## Generated labels and reserved names

The implementation uses `pds-<deployment UUID without hyphens>.subcult.tv` for PDS endpoints and `at-<retained invitation UUID without hyphens>.subcult.tv` for default account handles. Inputs must be canonical UUIDv4 values. Each generated label fits within a single DNS label. Customer display/server names never select routing labels. Endpoints stay bound to deployment identity when placement changes; account handles use their separate retained invitation identity.

All other first-level labels are outside this allocator, including management, OAuth, mail, existing PDS/pilot, brand, support and infrastructure names. Both generated prefixes are reserved for inventory allocation; matching a prefix does not establish that a name is allocated. The edge candidate generator emits exact inventory hosts only, rejects duplicates and verifies the pinned private-ingress certificate. It emits no wildcard route, on-demand certificate request, arbitrary upstream hostname or account-signup permission.

Initial issuance remains invitation-only under the existing grant/admission checks. One first-account invitation per supported PDS and the existing member ceiling do not become commercial plan limits. There is no public vanity-name search, arbitrary label reservation or automatic account creation. Any future friendly handle policy needs its own anti-squatting, abuse and retention rules. Pause new issuance independently of existing handle resolution. Do not reassign retained labels to another account or deployment.

The planned default handle proof is the HTTPS `/.well-known/atproto-did` method from the [AT Protocol handle specification](https://atproto.com/specs/handle). Only previously verified inventory bindings may be published; the account DID must also claim the handle. The generator is not a DID verification service and takes no customer HTTP requests. It can render a proof candidate for an operator-approved binding, but actual issuance/publication remains gated on account enrollment and bidirectional verification.

Existing proxied first-level wildcard DNS/edge service is the intended routing base, subject to fresh operator verification before publication. [Cloudflare's Universal SSL documentation](https://developers.cloudflare.com/ssl/edge-certificates/universal-ssl/limitations/) describes first-level coverage for a full setup zone. No deeper-name certificate purchase is selected. Public edge TLS and node-private TLS have separate checks and custody. A generated candidate neither changes DNS nor proves public DNS/TLS readiness.

The domain/Cloudflare DNS service decision is recorded. Registrar/account ownership evidence and the final publication/readiness qualification remain explicit open items; this document does not claim to have inspected the private registrar account. See [the #7 implementation sequence](managed-https-design.md).
