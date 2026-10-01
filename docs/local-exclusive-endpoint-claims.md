# Local exclusive endpoint claims

Status: locally implemented and verified on October 1, 2026. Positive public-domain reservation and endpoint activation remain unqualified.

An original current organization owner can POST an empty JSON object to `/api/organizations/{org}/endpoint-choices/{choice}/claim`. The exact saved choice UUID identifies the intent and receipt. GET at the same route returns only that owner's retained receipt. Management browser session, origin/CSRF for mutation and local mode are required; CLI bearer authority and production use are refused.

An unverified saved choice remains non-exclusive. A new reservation requires a fresh bounded observation of its exact TXT challenge through the service resolver. The store has its own eight-second deadline, and DNS retains the four-second/16-record/4096-byte bounds. Local `.test`, cancelled, expired and unconfirmed choices cannot obtain a new claim. No customer URL or resolver is accepted.

After lookup, organization/choice locks recheck current and original ownership, proof/payload identity, state, expiry, native hosting and pending handover. A hostname advisory lock serializes competing verified claimants and expiry reclamation. Recovery is checked again after waiting for that lock. A database unique index also prevents two reserved claims for one hostname. Reservation and its audit commit together; audit failure leaves neither.

Claims retain the proof digest and verification time. Database constraints bind the choice, organization, original actor and hostname and require an insertion within eight seconds of the retained verification time. Receipts recheck the current proof digest. A repeated exact request returns its retained receipt without another lookup or audit, including released or expired receipts. It cannot silently reopen them.

The reservation expires with the saved proof, within seven days. Reads project expired authority without writing an audit. A subsequent eligible claimant can reclaim the hostname by atomically marking the prior claim expired and writing a system audit. Its response does not expose the prior owner or receipt. Choice cancellation releases a reservation and audits release in the same transaction as cancellation. Retained choices, claims and receipts are preserved.

Claims reserve only the exact hosting hostname. They do not reserve declared application hosts. Every result keeps `canActivate=false`. No job, DNS record, route, TLS certificate, native endpoint, account identity or hosting permission is created by this feature. Activation will need fresh ownership and separate infrastructure qualification.

## Verification

The initial schema regression failed because schema 29 had no claim table. Focused race checks and the final full smoke gate passed. Tests cover database uniqueness, concurrent same-ticket retries, simultaneous competing verified owners, release, expiry reclamation/system audit, missing/mismatched proof, local/cancelled/expired/foreign/production paths, audit rollback, proof/ownership changes, recovery holds and a hold introduced during hostname-lock wait. Blocking DNS tests deny cancellation, expiry, ownership/proof change, a new native deployment and cancelled context during lookup. HTTP checks cover strict bodies, bearer refusal and production gating. The native-race fixture initially failed its managed-hostname NOT NULL requirement before reaching the claim behavior; after correcting its data, the final gate exercised and refused that race. Optional golangci-lint remains unavailable.

Management API/worker/monitor were paused for schema 30. All six prior endpoint-choice rows retained their exact values. Fresh recovery `6f30fe3f-a997-4d50-b594-b725985c4304` matched 53 tables and sequences, included empty claim/audit tables, revoked copied authentication and refused held worker startup. Reconciliation/replay preserved source management tables/sequences, default jobs and native signed data/media/custody. The initial capture report remains held; the separate replay report records readiness. This copy predates the later local refusal receipt.

Normal services resumed on schema 30 with ordinary fixture permissions off and admission open. Actual local refusal run `ca796f72-3602-4117-ad30-ccd9c2e94540` completed at 22:38:15Z. A new `.test` choice could not reserve a claim, repeated requests created no receipt/audit, and the cancelled choice still could not claim. Foreign scope, missing CSRF, customer resolver fields and bearer authority were denied. The created choice was retained cancelled, leaving seven choices/fourteen choice audits/zero pending choices and zero claims/claim audits. Only the intentional save/cancel and authentication activity changed management state; other work, native containers/start times, signed data/media and custody matched.

Private evidence remains in ignored `.local/`. No native tenant, occupied slot, export or recovery copy was deleted or reset. Earlier queued cancellation and browser work through PR #93 was observed merged into main; all phase/live issues remain open.

## Remaining gates

Add browser review, scoped receipt/status and exact-ticket transport retry. Positive public DNS matching/reservation is currently covered only by injected resolver/database tests. No positive populated-claim recovery or live routing/TLS/activation is claimed. A designated controlled domain and independent infrastructure remain necessary. DNS request budgets, resolver trust, revalidation, support policy and production qualification must be completed before claims authorize customer hosting.
