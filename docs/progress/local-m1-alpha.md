# Local M1 implementation progress

The user chose local build and verification first on 2026-09-30. No staging host, provider, public PDS domain or cloud budget has been selected. The foundation was merged in PR #31. Owner authentication was merged in PR #32. Durable deployments continue on `feat/durable-deployments`.

M1 depends on the M0 foundation. This branch begins with issue #1, then qualifies the disposable official PDS runtime from issue #4. Live infrastructure work remains pending. Neither local readiness nor a healthy PDS proves public federation or completes M1.

## Foundation evidence

- Pinned Go 1.27.1, Node 24.15.x and pnpm 12.6.0; committed dependency locks and a digest-pinned PostgreSQL image.
- `./scripts/dev.sh smoke` passed: frontend production build, Go vet and race-enabled tests against a new disposable database.
- Migration applies twice, rejects future schema version 999, and commits its version with the schema. The smoke database is isolated and removed afterward.
- Browser at `http://localhost:5173` showed database-backed readiness, then unavailable after stopping only `pds-ass-local-db-1`, then available after restart without reload.
- Measured overflow was false at 375×812, 768×1024, 812×375 and 1440×900.
- The first remote CI run failed because the runner uses Docker bridge networking without service DNS. The second found that its service-ID context is empty. The workflow now starts its own isolated, tmpfs-backed database and records the exact container ID for cleanup. [Gitea run 11325](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11325) passed the build, vet, race tests and clean-database journey on the Kvant runner. A simultaneous push/PR run then exposed a shared action-cache race in the runner. Feature branches now run on pull requests, with push checks reserved for main, to avoid duplicate runs for the same commit. [PR run 11328](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11328) passed after that change. [PR #31](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/31) contains the latest validation links and closes only #1 when merged.

## Official PDS evidence

- Official digest-pinned PDS reported version 0.4.5036. Caddy uses a separate digest pin.
- `./scripts/pds-local.sh qualify` passed health, unauthenticated admin rejection, invitation requirement, `.test` handle domain, unknown-host rejection (421), WebSocket upgrade, invitation persistence across restart, blob-volume canary persistence, and SQLite integrity for all three databases.
- Stopping the PDS produced a 502 through ingress; starting it restored the same checks.
- The PDS has isolated networking, separate persistent SQLite/blob volumes, generated secrets, CPU/memory/PID caps and no Docker socket. Caddy publishes only a loopback port.
- Actual account/record/upload fixtures, disk/account enforcement and database/storage fault injection remain pending under #4. A volume canary is not a repository blob test.

## Owner authentication implementation

- Added management schema version 2 with owners keyed by issuer and subject, hashed session tokens, expiring login attempts and owner-scoped organizations.
- Configured OIDC authorization code flow with PKCE, signed ID-token validation, browser binding, nonce comparison and atomic callback consumption. Production requires HTTPS and host-only Secure/HttpOnly cookies; HTTP is restricted to explicit local mode on loopback.
- Local/CI tests use an isolated signed test issuer with Alice and Bob. Neither test creates a live identity. Their matching email does not merge owners; the same subject at a different issuer also remains separate.
- `./scripts/dev.sh smoke` passed the authentication, organization isolation, expiry/revocation, API reconstruction and migration upgrade/rollback tests with the race detector.
- Browser verification signed in Alice, created Alice test studio, signed out, signed in Bob, proved Alice's organization absent, and created Bob test collective. Bob's session and organization survived an API restart.
- Signed-in layouts had no horizontal overflow at 375×812, 768×1024, 812×375 and 1440×900.
- Production provider baseline is a dedicated ZITADEL project using authorization code with PKCE. Live registration and provider qualification remain deferred alongside staging selection. This implementation does not use the existing Subcult applications or their identity databases.
- Authentication PR and remote CI links will be recorded in the PR after publication. Durable deployment intent is the next local slice under #5.

## Remaining local implementation

- Live production OIDC application configuration and provider qualification. Local owner identity, authorization and management sessions are implemented under #2.
- Restricted node enrollment and execution (#3, #6).
- Restricted agent integration with the durable deployment contract (#6).
- Managed hostname, enrollment, records/media and BYO handle flow (#7–#10).

## Deferred live qualification

Choose staging infrastructure and domains before public DNS/TLS, real email delivery, DID/relay/AppView visibility, cloud convergence and the timed 15-minute journey. Keep the corresponding issues open until their acceptance evidence exists.

## Durable deployment operations

- Schema version 3 stores organization-scoped deployment intent, request keys and progress events. River 0.47.0 jobs commit in the same transaction as intent and the first event.
- Identical requests return the same operation. Changed parameters or a duplicate server name return a conflict. Organization locking serializes concurrent submissions.
- A separate local worker checkpoints queued, configuring, checking and ready states. The test instance uses the operation ID as its primary key; repeated steps preserve one instance. Three attempts exhaust into a safe failed status. Cancellation preserves the checkpoint for retry.
- `./scripts/dev.sh smoke` passed build, vet and race tests. Database tests cover replay, conflict, foreign-owner reads, failed job insertion rollback, worker reconstruction, actual River processing, bounded failures, HTTP session/CSRF checks and the production submission gate.
- Browser verification submitted `restart-proof` while the worker was absent, observed queued status, started the worker and reloaded to the same ready operation. No horizontal overflow at 375×812, 768×1024, 812×375 and 1440×900.
- The browser lists stored operations and polls status. It retains unconfirmed request parameters in local storage for replay after a page closes. Local test readiness explicitly states that no PDS has been deployed.
- Hard-kill rescue timing, real host execution and public readiness remain subsequent qualification. No live deployment has been created.

## Restricted node agent qualification

- Added a separate local node-agent command with a TLS 1.3 listener requiring a CA-verified worker certificate. The API cannot reach the Docker socket through its own implementation.
- The allowlisted operation accepts only an instance UUID and an empty object. Fixed image, separate named data/blob volumes, internal network, CPU/memory/PID limits, dropped capabilities and no-new-privileges are agent policy. Existing foreign resources or altered container security/storage settings fail reconciliation.
- Private credential files survive agent reconstruction; atomic publication preserves the existing secret set. No deletion endpoint exists.
- `./scripts/dev.sh smoke` passed. Unit tests cover contract rejection, unauthenticated requests, secret persistence/symlink rejection, fixed Docker configuration and retry without another container.
- Actual local qualification created container `0dbad504a32c` for UUID `aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa`. Repeated requests before and after agent restart retained one container and exactly two labeled volumes. A fixed health probe returned HTTP 200 and version 0.4.5036. A TLS request without a client certificate failed.
- This is partial #6 groundwork. Automatic placement, measured capacity, one-time node enrollment, public ingress and worker-to-agent integration remain open. Container running status is not deployment readiness.
- [PR #33](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/33) holds the preceding durable operation slice; [CI run 11349](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11349) passed at commit `6bf1e74`.

## Local node enrollment and reservation

- Schema version 4 stores registered node endpoints, public-key hashes, public certificates, one-time bootstrap hashes and unique deployment placements.
- The separate local enrollment service signs ECDSA P-256 CSRs after checking a 15-minute, node-bound token. Consuming the token and activating the node commit together. CSR-selected roles and extensions cannot grant worker identity.
- The node retains its private key. A lost response can recover the existing public certificate after key/identity/trust verification, without issuing another certificate or accepting a replayed token.
- Database tests verify unknown/expired/replayed enrollment rejection, invalid CSR without token consumption, bounded certificate expiry and server-only role, plus concurrent reservation of a single slot and stable placement after node drain.
- Actual local CLI enrollment registered node `d795a58b-1ab4-4178-8649-03c9c25d0d9e` at the loopback agent endpoint with two operator slots. Repeating the client recovered the same public certificate after the server rejected token replay. Secrets stayed in ignored private files.
- Slot ceilings are not yet measured capacity, and registration does not prove the node's availability. Worker integration and verified measurements remain next. No provider or live node has been provisioned.

## Browser-to-agent local deployment

- Schema version 5 distinguishes simulations from local PDS operations and stores verified resource measurements. Existing simulations retain their behavior after configuring the agent worker.
- Worker-to-agent HTTPS validates the dedicated CA, registered node ID and public-key hash. Wrong keys and wrong node identities fail before runtime execution. The public API has no agent or Docker credentials.
- Placement uses fresh measured CPU/memory and retained namespace inventory, reserves host overhead and clamps to two local instances and the operator ceiling. Unregistered retained namespaces count against capacity. The agent independently checks its limit before creation.
- Reconstructed worker tests recover after host success and lost response with one instance ID, one placement and no duplicate progress events. Pending internal health uses bounded durable snoozes. Failed requests preserve reservations/storage; capacity errors remain explicit.
- Browser created `local-pds-one` as operation `bca4cb84-cfb2-4e58-afc6-26c5edad9ade`, resulting in one healthy container `7951dcce0fee`. A second request, `local-pds-two`, failed on capacity without another runtime. The older qualification fixture remained intact and counted against capacity.
- `./scripts/dev.sh smoke` passed build, vet and race tests, including real mutual TLS and public-key-pin checks. Responsive overflow remained false at 375×812, 768×1024, 812×375 and 1440×900.
- Ready means internal local PDS health only. Public ingress, real account fixtures, identity and federation remain unverified. The user reaffirmed local-only build/verification for later milestone preparation.

## Stable local ingress

- Schema version 6 allocates managed hostnames and local endpoint URLs before the job runs. They remain separate from node placement and cannot change silently during reconciliation.
- A pinned Caddy container serves each PDS through a read-only generated configuration, a loopback port and host allowlist. Admin access is disabled. Resource budgets include ingress overhead.
- The first local Caddy configuration had same-line block syntax that the actual Caddy parser rejected. The generated template was corrected, its local fixture updated and the pinned image validated the final configuration. Unit tests alone had not caught this parser error.
- `node scripts/check-ingress.mjs http://bca4cb84-cfb2-4e58-afc6-26c5edad9ade.localhost:44292` passed health/version, unknown host 421 and WebSocket upgrade. The collaborative browser reached the same hostname and displayed version 0.4.5036.
- Reconciliation retained the same PDS container, volumes and endpoint. New agent readiness combines internal PDS health with ingress health. This is partial #7 local evidence; public domain/DNS/TLS qualification remains deferred by the user's instruction.

## Isolated account and media qualification

- A separate local PLC/mail fixture uses the pinned upstream PLC implementation, persists operation history and mail captures, accepts only localhost identity endpoints and `.invalid` mail recipients, and rejects malformed or forged updates. No public identity operation or external mail delivery occurred.
- `./scripts/pds-local.sh qualify-account` passed fresh account creation and login, invite reuse rejection, forward/reverse handle and DID resolution, captured verification mail and confirmation-token reuse rejection.
- A single fixture post references an authenticated image upload. Record and blob reads match the written data. The WebSocket route delivers the matching commit. Upstream CAR validation checks block hashes and repository signature, rejects a wrong signing key and rejects a tampered export.
- Restarting the qualification PDS and protocol fixture retained login, confirmed email, DID document, record and media. Private fixture credentials remain in ignored `.local/account-fixtures/`; account data is retained for recovery qualification.
- The initial handle exceeded the upstream username limit, and the first PLC fixture route did not decode the upstream client's encoded DID path. Both were corrected. Email confirmation also required the upstream procedure's empty-body request rather than a JSON input. These checks exercise the actual pinned runtime.
- `./scripts/dev.sh smoke` passed frontend build, Go vet and race tests. Basic runtime qualification also passed SQLite integrity, invite/blob-volume persistence and stopped-runtime failure detection after populated account qualification.
- Product enrollment expiry, wrong-tenant invitations, account/disk limits, public DNS/TLS, approved live email and downstream relay/indexing remain open. This does not complete #4, #8 or #9.
- [PR #37](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/37) passed [CI run 11362](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11362) at `22ea5ca`.

## Fail-closed local registration qualification

- The pinned PDS has no native invitation expiry field. A local ingress guard now checks a tenant-bound lease at registration admission for both supported XRPC and OAuth signup routes. It receives no password body. Registration stays closed for invalid/expired policy or unavailable guard.
- The qualification broker closes registration before revoking unused old codes, checking its 32-repository fixture ceiling and publishing a new 15-minute lease with one-use native code. Durable policy publication and serial qualification prevent accidental overlapping runs.
- Actual qualification rejected expired, wrong-tenant, malformed, future-issued and excessive leases. Case, trailing-slash and encoded XRPC aliases also failed closed. Stopping the guard blocked registration with 502 while PDS health stayed 200. Recovery preserved the lease. Renewing an invitation revoked the previous unused code, and used-code replay failed.
- The fixture account journey still passed identity/email, signed repository/media, stream events and restart persistence with the guard enabled. Product owner authorization, bounded recovery, persisted enrollment status and agent guard integration remain open for #8. The fixture ceiling is not a shared-hosting account limit.
- Managed local PDS health currently proves transport only. Upstream derives its advertised PDS URL from hostname/port configuration; the agent's advertised HTTPS URL on port 443 must be reconciled with its current HTTP ingress on a reserved local port before creating identities there. Its PLC URL remains disabled, so no incorrect public identity was created.
- `./scripts/dev.sh smoke` passed build, vet and race tests for this slice. The actual pinned Caddy binary validated the guard configuration.

## Managed local TLS and advertised endpoint

- Schema version 7 preserves managed hostnames/ports while correcting local agent endpoints to HTTPS and recording the correction in operation history. The pinned PDS's advertised OAuth issuer and service DID now encode the same hostname and reserved port. Public identity access remains disabled.
- Instance-specific seven-day TLS identities are retained in private atomically published files. Health checks trust only the instance certificate with TLS 1.3 and exact hostname verification. Unit tests reject missing trust, another hostname, unsafe permissions and symlinks, and retain the same identity after reconstruction.
- Actual empty fixture `bca4cb84-cfb2-4e58-afc6-26c5edad9ade` was stopped and archived before correction. Both old containers, its private inspection snapshot, old Caddy configuration, volumes and runtime credentials were retained. No account existed on this managed fixture. Populated identities are refused by the helper.
- New runtime reconciliation fences stopped prior writers before reusing the same namespace. Tests reject running, paused and restarting writers. Capacity still counts retained namespace IDs once.
- Initial TLS reconciliation failed safely. Inspection and repeated qualification exposed unordered Docker mount responses; exact source/destination set validation now replaces positional validation. A regression test reverses mount order. Stopped-ingress reconciliation and agent reconstruction retained the certificate and recovered healthy ingress.
- `scripts/check-ingress.mjs` passed TLS, advertised issuer/service DID, versioned health, unknown-host rejection and WebSocket upgrade at `https://bca4cb84-cfb2-4e58-afc6-26c5edad9ade.localhost:44292`. The pinned Caddy binary validated its final configuration. The browser displayed the corrected HTTPS health link without overflow. No system trust root was installed.
- Owner-authorized managed enrollment, local identity/mail integration, certificate renewal, public DNS/TLS and federation remain open. This corrects local transport identity before enrollment; it does not complete live #7 or #8.
- [PR #38](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/38) passed [CI run 11375](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11375) at `5f373f9`.
- Final `./scripts/dev.sh smoke` passed build, vet and race tests, including the version-6 endpoint data migration and its single audit event. The migration regression fixture initially omitted a required organization UUID; this test setup was corrected before the final gate.
- [PR #39](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/39) passed [CI run 11376](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11376) at `59442e8`.

## Encrypted populated-PDS recovery

- Added an operator-only, fixed local recovery drill and authenticated AES-256-GCM envelope. Fresh per-drill keys stay separate from encrypted bundles, and decryption validates its tag before publishing plaintext. Tests reject tampering, wrong keys, truncation, unsafe keys and overwrite attempts. The smoke gate now runs these Node tests.
- Source writers stop before the complete volume trees and journal companions are copied into fresh staging volumes. SQLite's backup API produces portable copies there; every captured database passes integrity checking. Source databases, containers, volumes and credentials remain intact. Initial read-only WAL inspection failed safely; staging normalization resolved the companion-file requirement.
- The final actual drill captured 38 files and 13 SQLite databases. All 9 repository signatures, 15 records and 8 referenced blob CIDs/bytes matched after restoration onto fresh storage. The designated account retained login, confirmed email, media and identity, then wrote a signed record with a matching stream commit.
- Recovered PLC material signed a temporary rotation in the restored isolated directory, and the new key signed rollback to the original public rotation set. No public identity operation occurred.
- The final local recovery completed in 8.43 seconds, with zero observed source-marker loss and 1.64 seconds between the designated marker and capture. This is a small cold localhost drill, not a scheduled RPO or production RTO guarantee. The report is `.local/recovery-work/8dc4e83f-760f-4dc1-bebd-fab918db4d86/report.json`.
- Restored writers stopped before the original stack resumed. Restored/staging volumes and containers remain for review. Independent/off-machine custody, retention, management recovery, agent/CA/TLS material, production sizing and live identity/email/federation remain open for #11 and #12. See [local recovery](../local-recovery.md).
- `./scripts/dev.sh smoke` passed Node encryption tests, frontend build, Go vet and race tests. [PR #40](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/40) passed [CI run 11379](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11379) at `e721d70`.

### Management recovery hold and read-only host inspection

The management schema now includes a recovery hold ledger. API and worker startup refuse a held database, and each worker checkpoint checks the ledger before host execution. A held or unavailable ledger snoozes the job without advancing its deployment or exhausting retries. The operator command accepts only loopback databases named `pds_recovery_` followed by 32 hexadecimal characters. It can record an authenticated snapshot digest, inspect the hold, and reconcile registered hosts before release.

Reconciliation authenticates each enrolled node through the existing CA, node identity and registry key pin. It compares all retained placements against host namespace inventory and requires every placed instance to be running and healthy at its stable endpoint. Missing, duplicate or untracked namespaces preserve the hold. It does not start containers, recreate credentials, delete resources or advance deployment checkpoints. Existing local API, worker and agent processes still run the preceding compiled version until the recovery qualification restarts them.

Regression checks cover an unavailable ledger, refusal to hold a source database, held jobs making no provisioning calls, stopped and absent containers, missing credentials remaining absent, mismatched runtime configuration, ambiguous inventory and successful reconciliation. The complete PostgreSQL dump, encrypted restore, uncertain-job replay and runtime restart qualification remains pending. This is groundwork for issue #13, not its acceptance evidence.

The first encrypted management restore retained 2 owners, 3 organizations, 3 deployments, 12 deployment events, 1 placement and 1 enrolled node. Disposable copy `pds_recovery_afd8ea8687f14614bb4c2cf5e08b529e` remains held. Its worker refused startup before execution. Source and restored inventory counts matched. The restarted local agent's read-only endpoint confirmed the placed TLS instance was healthy. Reconciliation then correctly refused release because the agent retains the earlier manual `aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa` namespace, which has no management placement. No runtime or source database was deleted or modified by reconciliation. Explicit accounting for retained unmanaged fixtures and uncertain-job replay remain pending.

A second restore, `pds_recovery_e70adda4f4304726a2ddd677b53e7779`, strengthened comparison to SHA256 hashes of every restored row in the six management inventory tables, as well as their counts. All hashes matched the source before and after worker startup refusal. The restored copy remains held. The full local smoke gate passed after these changes.

### Recovered management job replay

The restored `pds_recovery_e70adda4f4304726a2ddd677b53e7779` copy reconciled after an explicit ledger acknowledgement of the earlier unmanaged local qualification fixture. Managed deployment IDs cannot receive that exemption; new holds clear acknowledgements, absent fixtures fail and acknowledgement itself leaves the database held. The placed runtime passed authenticated read-only status inspection.

The drill reconstructed a lost `checking` checkpoint in the disposable copy and inserted a new River job for the same deployment UUID. A real worker completed it on attempt 1 and restored the deployment to `ready`. PDS and ingress container IDs stayed unchanged, as did protected credentials and TLS identity hashes. Source management deployment rows stayed unchanged. Private evidence is in `.local/management-recovery/e70adda4-f430-4726-a2dd-d677b53e7779/replay-report.json`. The complete local smoke gate passed. PR #42 CI run 11385 succeeded for exact SHA `419d4476750ac978c3663a59ef43afb9813cd811`.

This qualifies local snapshot restoration, hold enforcement, host reconciliation and uncertain-job replay for the retained fixture. Independent off-machine storage, separate credential custody, real process termination recovery timing and production RPO/RTO remain unverified.

### Independent local service observations

The separate monitor now records bounded host, capacity, runtime, queue and recovery findings through authenticated read-only agent inspection. Duplicate subject/kind findings keep one row, resolved findings retain timestamps, and failed scans cannot clear prior evidence. Owner-authorized deployment responses include current, degraded, unknown or stale service observations and explicitly report that scheduled backups are unconfigured. Observations expire after one minute. The management API never acquires agent credentials or Docker access.

The actual outage drill completed at `2026-10-01T06:51:10.582Z` with the management API stopped. Stopping the placed fixture's ingress produced one active runtime outage across repeated scans. Restarting it resolved the finding and preserved its container ID. The API and worker now run schema 10; the source deployment rows remain preserved. The complete smoke gate passed, including stale-status and atomic/deduplicated finding regressions. The browser showed the owner's healthy observation on desktop and phone at 375 by 812 without horizontal overflow. The T3 preview host disconnected before fresh tablet and landscape checks could complete.

`docs/local-monitoring.md` contains the foreground scan commands and local incident procedure. No scheduler or alert delivery destination has been installed. External HTTPS/WebSocket probes, disk thresholds, certificate-expiry monitoring, stale-backup alerts backed by a real schedule and a live on-call owner remain pending for issue #14.

### Managed first-account admission boundary

The retained empty managed fixture was prepared for isolated local PLC and captured SMTP while preserving its UUID, endpoint, data/blob volumes, credential file and TLS identity. Prior identity-disabled PDS and ingress containers are stopped and archived with the suffix `before-local-identity`; their private inspection and TLS digest evidence is in `.local/local-identity-upgrades/`. The agent now runs with explicit local protocol-fixture mode. Production identity configuration remains unavailable.

The placed PDS has no direct published port. Its managed Caddy ingress checks native XRPC and OAuth-provider signup paths against a separate tenant-specific enrollment guard. The guard has fixed image/command, internal networking, read-only private state, a read-only root filesystem and 64 MiB/one-eighth CPU limits. Capacity measurements reserve that additional cost. The normal API and worker still have no host administration capability.

The actual ingress drill completed at `2026-10-01T07:14:55.834Z`. Wrong-tenant, expired, future and overlong leases were denied across case, encoded-path and trailing-slash aliases. A current lease reached native signup validation. Stopping the guard denied registration while PDS health remained available. Cleanup restarted the same guard and left its lease closed. Public fixture-certificate checks confirmed the unchanged HTTPS issuer/service identity and WebSocket endpoint. Authenticated read-only agent status again reported the PDS healthy. Docker Desktop's brief bind-mount visibility delay required publication verification inside the isolated guard; restart readiness is bounded and checked.

The complete local smoke gate passed with tenant-bound lease, unsafe-file, isolated-transport, capacity and foreign-fixture regressions. Owner-authorized invitation jobs, first-account completion and recovery remain pending for issue #8. No live email, public trust or federation acceptance is claimed.

### Fixed native first-account invitation broker

The restricted agent now accepts a fixed enrollment ticket through authenticated worker transport. Native administrator operations execute only inside the existing isolated PDS container. No administrator key leaves that runtime, and no API request can provide commands, images, paths, environment or account passwords. Helper output is framed, bounded and validated; private errors are replaced with a generic failure.

Stable ticket retries preserve the native code and expiration. Replacement closes and confirms the admission policy, drains pending native signup requests, recounts all actors and revokes prior unused codes before issuance. Expired retries cannot extend the lease. Recovery from a lost native helper response remains closed until replay completes. Native account completion closes the lease; the guard independently rejects signup once native repository inventory is populated, including when management workers are unavailable. The earlier guard container was retained stopped for rollback when the fixed ceiling policy changed.

The actual broker drill completed at `2026-10-01T07:33:35.163Z` for the placed fixture. Current-ticket replay stayed stable, the replaced code was rejected by native signup, the expired code was denied at ingress, unsupported configuration was rejected and administrator/JWT/PLC-rotation credentials were absent from responses. No account was created. Private code and report files remain in `.local/enrollment-broker/`; enrollment is expired. Regression tests also cover lost helper responses, no lease extension, bounded helper frames, private-error rejection and no second invitation after account completion. The owner-authorized management job and browser flow remain pending for issue #8.

### Owner enrollment jobs and native account completion

Schema 11 adds owner-scoped enrollment intents, fixed audit messages and durable River jobs. Submission and job insertion are atomic; same-key replay preserves the native ticket and expiry. Recovery is explicit and bounded to three requests per deployment per hour. Refresh uses a five-second admission interval. Foreign owners receive 404, mutations require the owner session, allowed origin and CSRF token, and unexpected password fields are rejected.

Transient invitation codes are encrypted with a private management key and operation/deployment/organization authenticated scope. They appear only in an authorized, non-cacheable response and React memory; browser storage contains retry identifiers only. API code never loads the host client credentials. Worker recovery holds prevent native execution, and a failed management checkpoint replays the same native ticket without issuing another code.

The actual owner-to-native journey completed at 2026-10-01T08:12:53Z on managed fixture `bca4cb84-cfb2-4e58-afc6-26c5edad9ade`. Initial submission replay and foreign-owner denial passed. Authorized recovery replaced the code. The initial old-code request used an overlong handle, so its 400 response does not establish code revocation. The separate native broker drill covers valid-handle old-code rejection. The account was created directly through its retained TLS endpoint. Registration then returned 403 before management refreshed its invitation checkpoint. Captured email confirmation, rejection of a reused confirmation token, native account login and management's `verified` state all passed. The local PLC document retained the same HTTPS service endpoint. Credentials remain private; the secret-free report is `.local/owner-enrollment/3ef3731b-9684-4dd8-926e-1d14f93ea1e0/report.json`.

The first attempt exposed the pinned PDS's 18-character service-handle limit; the script shortened the fixture prefix without creating another enrollment operation. A later empty-body request mismatch interrupted email confirmation after account creation. The script resumed using the retained account credentials and completed confirmation. Neither failure reset storage or changed identity. Browser guidance now states the handle limit. The owner page shows confirmed email, hides the invitation and has no phone-width overflow.

`./scripts/dev.sh smoke` passed the frontend build, Node checks, Go build/vet and all race tests, including real disposable PostgreSQL owner authorization, concurrent idempotency, encryption and checkpoint replay. See [local first-account operations](../local-owner-enrollment.md). Live mail, DNS, TLS, federation and the full #8 release acceptance remain open.

### Customer handle instructions and verification groundwork

Schema 12 stores owner-scoped domain intent, revisions and fixed audit events. Verified account owners receive exact TXT instructions. The API does not receive native passwords or session tokens and does not perform account changes. The native authenticated account flow remains the account-control boundary. DNS checks reject ambiguous, malformed and conflicting DID records, require a matching reverse alias and preserve the exact hosted service endpoint. Customer hostnames are never fetched as HTTP URLs.

Rechecks are bounded to 30 seconds across domain changes; stale in-flight results cannot overwrite newer intent. Tests cover unauthorized and foreign owners, CSRF/origin rejection, overlong bodies and credential fields, unconfirmed accounts, DNS propagation/removal, reverse alias absence, duplicate/conflicting services, directory bounds and redirects, and stable deployment state. The full local smoke gate passed.

Actual management qualification completed at 2026-10-01T08:25:39Z. Exact `_atproto.creator.handle.test` instructions, owner scope, idempotent configuration, credential-field rejection, bounded checking and pending DNS passed, with deployment state and endpoint unchanged. Report: `.local/handle-proof/5b6fd80e-d35d-4820-a69c-f745effd2f42/report.json`. This is instructions and pending-DNS evidence, not an attached controlled domain. Native custom-domain attachment/removal, well-known support and browser qualification remain open. See [handle proof](../local-handle-proof.md).

Owner enrollment [PR #47](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/47) passed [CI run 11396](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11396) at `85b7246`.

### Measured local capacity policy and second owner

Schema 13 records local operator capacity changes with actor, fixed reason, previous ceiling and requested ceiling. The loopback-only `node-admin capacity` command invalidates fresh placement measurements after a change. It does not evict existing namespaces or expand measured capacity. Agent policy defaults to two instances; an explicit ceiling between one and eight still reserves two GiB and one CPU for host overhead, then accounts for every tenant's PDS, ingress and enrollment guard. Tests cover CPU/memory clamping, invalid policy, idempotent audit, unregistered nodes and fresh-measurement fencing.

The actual local Docker VM reported 8 CPUs and 8,321,798,144 bytes. An audited three-slot ceiling enabled a second managed deployment while retaining the earlier manual namespace. Bob's separate organization provisioned `8d234722-f0ad-47b2-8ef8-10f85d0c13b9` at `https://8d234722-f0ad-47b2-8ef8-10f85d0c13b9.localhost:42131`. Its owner journey completed at 2026-10-01T08:33:07Z, including a valid-handle replaced-code rejection, native account creation and email confirmation, admission closure and unchanged PLC endpoint. Its report is `.local/owner-enrollment/f2b4432f-6047-47d2-9646-b8f892193757/report.json`. No volumes, earlier fixtures or unrelated containers were deleted. Populated cross-tenant and crash qualification remains the next gate.

The handle-instruction qualification was rerun with an explicit nonempty `endpoint` comparison, correcting an earlier test's unused `endpointUrl` field. Report `.local/handle-proof/c9aec412-6e3f-4353-8c38-3de1535f4817/report.json` passed at 2026-10-01T08:32:32Z. Handle proof [PR #48](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/48) passed [CI run 11397](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11397) at `c5b6727`.
