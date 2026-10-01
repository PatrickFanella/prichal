# Planning a custom PDS address locally

An owner can check a proposed PDS hostname and up to eight declared application/OAuth hosts from the organization page. The check saves no choice and reserves no domain. It performs no DNS lookup, certificate request or host operation. Existing native hosting retains its endpoint and receives a migration/replacement review warning.

`POST /api/organizations/{id}/endpoint-preflight` requires the current owner's management browser session, Origin and CSRF checks. Members, former owners and CLI credentials have no authority. The route is local-only; production returns unavailable. It accepts one strict JSON object within 4096 bytes and returns fixed planning fields without private native information.

Names must be ASCII DNS hostnames, including validated A-labels. URLs, ports, IP addresses, trailing dots, reserved namespaces, unknown suffixes and public suffixes alone are refused. `.test` is permitted only locally. Case normalizes to lowercase, and declared app hosts sort and deduplicate. This syntax check establishes no domain ownership.

The pinned `golang.org/x/net` public-suffix snapshot compares registrable sites, including multi-label suffixes. The configured management origin participates in separation except for local localhost/loopback origins. Sibling PDS and app subdomains under one registered domain fail. This follows the AT Protocol [production domain guidance](https://atproto.com/guides/going-to-production); the [publicsuffix package](https://pkg.go.dev/golang.org/x/net/publicsuffix) supplies the boundary rather than a final-two-label guess. Owners must declare future app/OAuth hosts too.

Every successful result has `canActivate=false` and `choiceSaved=false`. Ownership proof, supported routing, TLS, a saved choice and supported provisioning remain required. Organizations with retained native hosting also require a separately reviewed migration/replacement plan. A successful result has no deployment authority.

## Local qualification

The full smoke gate passed Go vet/race/disposable PostgreSQL tests, Node checks and the frontend build. Tests cover hostname normalization, registrable-domain separation, canonical declarations, current/foreign/former ownership, retained-native warnings, strict JSON/body bounds, real route scope and the production gate. `x/net` is pinned to v0.59.0; its module requirements also upgrade `x/sync` to v0.23.0 and `x/text` to v0.42.0. Toolchain pins remain unchanged. Optional golangci-lint remains unavailable.

Actual read-only drill `9b6f6eca-5b11-4f31-ad25-c2ad0902ebfa` completed at 2026-10-01T18:34:59Z. Both populated native organizations received the review warning, and an empty organization received planning-only guidance. Repeat, invalid-input, domain-conflict, strict JSON, foreign scope, missing-CSRF and CLI denial checks passed. Deployment endpoints/hostnames, enrollment policy, audits, jobs and enrollment rows remained unchanged. Native signed repositories/media, container identity/running state and protected custody hashes also matched.

Actual keyboard submission at 18:35:42Z normalized the empty organization's address and focused the result with a visible outline. Editing to a sibling app domain cleared that result and focused the rejection at 18:36:39Z. The populated organization showed the migration warning at 18:36:44Z and retained its original native health link. A temporary read-only unavailable-response fixture cleared the result, preserved inputs, focused the error and cleared busy state at 18:40:59Z. Original fetch and actual data were restored. An earlier fixture attempt found no buffered ResourceTiming route; the repeat used the already observed local route.

Initial phone checks found form-internal overflow despite no document overflow. The grid/input minimum-width fix passed a fresh frontend build. Final checks at 320×568, 375×812, 812×375, 768×1024 and 1440×900 found no document/plan overflow or controls outside the panel. Inputs have labels. Private metadata remains in `.local/endpoint-preflight-browser.json`. T3 snapshot capture failed, so this does not establish a new screenshot or assistive screen-reader qualification.

The schema remains 25. The existing 47-table recovery checkpoint remains applicable because preflight writes no management state. Local API/worker/monitor retain default fixture permissions disabled and both admission policies open. No native container, volume or recovery copy was removed.

## Remaining work

Save a reviewed owner intent before account enrollment, prepare and verify scoped domain ownership, and qualify supported routing/certificates/provisioning on designated infrastructure. An unverified choice must not reserve a domain globally or silently deploy the default managed address. Existing account endpoint changes need their own migration plan. Public DNS/TLS, custom activation and live deployment timing remain unqualified.
