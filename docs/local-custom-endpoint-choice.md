# Saving a local custom-address choice

Schema 26 adds a reviewed owner intent for a new custom PDS endpoint. The management API can save, read and cancel it. Browser mutation controls are the next slice; the current browser still exposes read-only preflight. Production remains unavailable and the customer CLI has no endpoint-choice authority.

The local routes are:

- `POST /api/organizations/{id}/endpoint-choices` with hostname, declared application hosts, stable `idempotencyKey` and `confirmed=true`.
- `GET /api/organizations/{id}/endpoint-choice` for the pending choice or null.
- `GET /api/organizations/{id}/endpoint-choices/{choice}` for a retained receipt.
- `POST /api/organizations/{id}/endpoint-choices/{choice}/cancel` with a stable cancellation key and fresh confirmation.

All routes recheck current owner access; mutation requires browser Origin/CSRF. Retained receipt reads also require the original request owner. Strict JSON rejects unknown fields, trailing values and bodies over 4096 bytes. Fixed errors omit database/host details. Management recovery holds refuse saves/cancellations, including mutation retries.

Save reuses [preflight](local-custom-endpoint-preflight.md) normalization and registrable-domain separation. Hostnames are limited to 244 characters to leave room for a TXT proof label. A 32-byte random challenge produces `_pdsaas.<hostname>` with a `pdsaas-domain-v1=` value and seven-day expiry. These are instructions for later ownership verification. No DNS lookup, routing, certificate request or native provisioning occurs; `canActivate` is always false.

One pending choice per organization serializes under the same organization lock as native deployment and ownership handover. Identical save retries return the same proof and current state. Changed payloads conflict. Cancel records one atomic audit, retains the receipt/proof and requires the original cancellation key for replay. Retrying a cancelled save never resurrects it. A new reviewed save generates a new proof. Each organization retains at most 30 choices; no receipt is deleted to regain capacity.

Unverified hostnames have no global uniqueness constraint. Other organizations may prepare independent proofs for the same address. A future verified claim must establish ownership and enforce its own conflict rules before activation.

Any retained native deployment prevents a new choice. A pending choice, including expired proof, blocks new default native deployment and both offering/accepting organization handover. Expiry never silently selects default hosting. Matching committed deployment retries precede this check; simulations remain independent. Cancellation does not queue a deployment. Default hosting requires a separate reviewed request after cancellation. No existing endpoint or identity changes.

## Qualification

Full smoke passed Node checks, frontend build and Go vet/race/disposable PostgreSQL tests. Tests cover canonical/stable requests, changed payloads, foreign/former ownership, cancellation replay, audit rollback, recovery holds, retained request caps, TXT-name bounds, independent same-hostname intents, pending handover and strict HTTP/production boundaries. Concurrent save retries produce one audit. A native submission/save race waits on the organization lock and commits exactly one competing intent with matching job count.

Actual drill `5f4427be-f0e0-40e7-9955-6cf3314e1c94` completed at 18:56:06Z. Two choices were saved and cancelled sequentially with four audits. The qualifier was then tightened to hold both identical-hostname choices pending at once and recover a lost response only by replaying its exact request. Repeat drill `014cb4d9-9c0d-46b5-8be3-4a70548690a7` completed at 18:58:13Z. Both independent proofs coexisted, stable reads/retries matched, changed/foreign requests and missing-CSRF/CLI authority were denied, and pending native submission was refused. Both choices were cancelled. Across both drills, four cancelled receipts/eight audits remain; management work, deployment endpoints, admission policies, native signed data/media, containers and protected custody stayed unchanged.

Fresh schema-26 recovery `24489b6d-dbb4-4315-9359-9ba2402428f1` completed at 18:58:36Z and matched all 49 tables/sequences, including four retained choices/eight audits and 79 jobs. Copied authentication was revoked and held worker startup refused. Reconciliation/dedicated replay preserved copied default work, source rows/sequences and native custody. Source API/worker/monitor were paused across capture/replay and resumed afterward. All copies remain retained. An earlier schema-26 copy containing the first two receipts also passed and remains retained.

Browser save/reload/retry/cancel behavior remains unqualified. Domain ownership verification, globally exclusive verified claims, supported routing/TLS/provisioning, existing-account migration and live timing remain open. No native container, volume or recovery copy was removed.
