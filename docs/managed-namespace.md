# Managed namespace decision — October 28, 2026

The owner selected first-level names under `subcult.tv`, with no domain purchase, for #126. Reuse the existing domain and Cloudflare DNS/edge service. Public NS lookup on October 28 returned `dahlia.ns.cloudflare.com` and `hasslo.ns.cloudflare.com`; this confirms delegation, not registrar or account access. The existing domain custodian retains control; no registrar transfer, certificate purchase or zone-wide DNS permission is implied.

The #7 design must keep managed endpoint and default-handle labels one level below `subcult.tv`, preserve management/OAuth and existing service/pilot labels, and reserve separate collision-safe prefixes for endpoints and accounts. Exact prefix allocation, reserved names, handle issuance/abuse limits and inventory-only routing must be specified and verified before name issuance. Nested `*.pds.subcult.tv` is outside this decision.

This records the chosen namespace. It changes no DNS, certificate, route, PDS identity or signup policy. #126 remains open for its remaining design/ownership evidence; #7 remains gated by #6 and actual DNS/TLS/WebSocket/failure qualification. The hosted #126 and #7 issue bodies record the same decision.
