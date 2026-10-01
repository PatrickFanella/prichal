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

### Populated managed tenants and abrupt runtime failure

The actual two-tenant drill completed at 2026-10-01T08:37:53Z. Alice and Bob each wrote a record with uploaded media. Both CAR exports passed CID/MST and repository signature verification against their local PLC keys, with matching subscription commits; wrong keys and corrupted CAR bytes were rejected. Private management reads returned 404 across owners. Native cross-tenant write tokens returned the pinned runtime's 400 `InvalidToken` response. The first drill expected HTTP 401; it stopped before any crash action, and the corrected check now requires the authentication error itself.

Both PDSs retained separate UUID volumes and private internal networks, no native host ports, one CPU, 768 MiB, 128 processes, dropped capabilities and no added privileges. Direct neighbor PDS and link-local metadata probes failed. Bob's PDS was killed abruptly; Alice logged in repeatedly and served its signed record and media while Bob was stopped. Bob restarted in the same container and retained confirmed login, signed record and blob bytes. Both credentials, TLS certificates and container IDs matched their pre-drill identities. Report: `.local/tenant-isolation/44eaa364-4082-407f-8289-96515574aeea/report.json`.

The drill retains both accounts and volumes. It does not claim sustained resource-pressure behavior, aggregate storage quotas, production egress or production isolation. Shared development PLC/mail capture remains trusted fixture infrastructure, not a customer-isolated service. See [local tenant isolation](../local-tenant-isolation.md). Issue #16 remains open.

### Native usage and retained measurements

Schema 14 adds per-instance usage observations. The authenticated, pinned agent client invokes only a fixed embedded helper in an existing healthy policy-validated PDS. Traversal is restricted to the two native volume paths, with symlink, depth, entry-count and integer bounds. It returns counts and logical bytes, never private contents, account identities or file names. Shared filesystem headroom stays with the operator monitor.

Monitoring now records unavailable usage, shared filesystem pressure and an exceeded one-account ceiling. Measurement persistence and findings commit atomically; failed probes preserve counters and prior pressure findings, and older samples cannot replace newer ones. Owner-scoped deployment responses expose unknown, observed, unavailable or stale counters and explicitly unconfigured total storage limits. The browser shows usage freshness. Tests cover native counter redaction, unsafe traversal, mismatched/stale/overflowing measurements, pinned access, atomic persistence, observation outages and pressure recovery. `./scripts/dev.sh smoke` passed frontend build, Node tests, Go build/vet and race tests.

Actual Alice/Bob observations returned positive native storage and blob bytes and one account each. The controlled Bob ingress outage drill completed at 2026-10-01T08:53:28Z. Foreign-owner reads returned 404, host headroom was hidden, unavailable usage retained the exact prior counters/time, and renewed observation kept the endpoint and ingress container unchanged. Report: `.local/usage-qualification/40af7337-920d-4f2e-813a-36e050f9bd4e/report.json`. Neither native PDS was stopped for this usage drill. Monitoring watch resumed afterward. See [local usage](../local-usage.md). Storage quota enforcement and billed aggregation remain open.

Capacity policy [PR #49](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/49) passed [CI run 11400](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11400) at `acc6947`. Populated isolation [PR #50](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/50) passed [CI run 11401](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11401) at `fc2717f`.

### Encrypted populated managed-tenant recovery

The managed local recovery qualification validates owner scope and fixed runtime policy, captures native data/blobs, per-instance credentials, TLS identity, ingress/registration policy and the shared local PLC/mail fixture. Source writers stop before capture. Normalization and integrity checks operate on new staging copies. The authenticated encrypted bundle restores onto fresh volumes and a fresh private network at the same local endpoint; the guard sees only its dedicated lease directory, not protected runtime metadata.

The final actual drill completed at 2026-10-01T09:04:20Z. It captured 15 files and 5 SQLite databases totaling 620,897 bytes. One signed repository, 4 records and one referenced blob matched after restoration. Confirmed native login and the retained TLS endpoint passed. A new restored write produced a verified repository signature. The recovered PLC signer performed temporary key rotation and rollback solely in the restored directory. Restored writers stopped before the original resumed; its signed data, native container and source identity document remained intact.

The final measured cold drill took 7.087 seconds on this tiny localhost fixture. Report: `.local/managed-recovery/e597f6a7-f5cb-4666-8141-d85a9e83df52/report.json`. The first managed drill also restored successfully, then the final drill added signed-write and identity-rotation verification. Both retain stopped fresh/staging resources and separate private encryption keys. The shared fixture snapshot contains other development identities/mail and is operator recovery material, never a customer export. Independent custody, scheduling, retention, realistic data sizes, host/management keys and live recovery remain open. See [managed tenant recovery](../local-managed-recovery.md).

Native usage [PR #51](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/51) passed [CI run 11402](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11402) at `f72b6c8`.

### Failed candidate and untouched-source rollback rehearsal

The managed recovery helper now accepts only a fixed `--candidate-health-fault` option. The candidate runs against fresh restored volumes, with a fixed exiting command in the pinned image. The original remains fenced and untouched until candidate writers stop. The report distinguishes failed staging from a successful restored runtime and never claims a distinct release was qualified.

Actual Bob rehearsal completed at 2026-10-01T09:12:49Z. The encrypted snapshot retained 15 files and 5 SQLite databases. The candidate failed health checks; its writers stopped before the original resumed at the same endpoint. Its 3 signed records, referenced blob, native container and source identity remained intact. Report: `.local/managed-recovery/c496de19-21af-4d17-a319-d7f838220e7e/report.json`. The 3.703-second local measurement is a small failure rehearsal, not a migrated-data rollback or production RTO claim.

A current registry inspection resolved `pds:0.4` to our existing immutable digest. No distinct release was selected. Reviewed release/schema compatibility, canary promotion, lifecycle serialization and real cross-version rollout remain open for #15. See [release rehearsal](../local-release-rehearsal.md).

### Audited local service suspension and resume

Schema 15 adds durable operator service operations and audit events. The loopback-only `service-admin` command records fixed reasons, OS operator identity, prior state and generation. Intent, audit and River job commit together. Customer HTTP has no operator mutation route. The restricted agent records a private hold before stopping the retained ingress, registration guard and PDS. Ordinary reconciliation and enrollment respect the hold. Resume confirms existing policy and health; older or conflicting generations fail. Management recovery now requires exact confirmed host lifecycle metadata and preserves intentional suspension.

`./scripts/dev.sh smoke` passed frontend build, Node checks, Go build/vet and race tests. Coverage includes concurrent idempotent intent, uncertain checkpoint replay, management recovery holds, retained unknown failure state, interrupted Docker stops, stale generations and intentional suspension monitoring. Optional golangci-lint remains unavailable.

Actual Bob qualification completed at 2026-10-01T09:36:24Z. All three tenant processes stopped and reconciliation did not restart them. Alice remained available. Bob resumed with original container IDs, credentials, TLS/admission policy, confirmed account, signed records and media. Stale suspension failed; foreign-owner reads and customer escalation returned 404. Report: `.local/service-lifecycle/f01c57c5-3eba-421d-a4a5-30e4ea88f96a/report.json`.

A subsequent encrypted managed recovery captured the private running lease alongside native data and host metadata. It completed at 2026-10-01T09:37:55Z with 16 files, five SQLite databases, four signed records and one referenced blob. Fresh writes and recovered PLC signing passed; the untouched original resumed after restored writers stopped. Report: `.local/managed-recovery/f3c0b25d-a048-445d-9aad-2a80110658c8/report.json`. Held restoration/export, native account takedown, customer notification and live lifecycle qualification remain open for #17. See [local service lifecycle](../local-service-lifecycle.md).

### Audited native account takedown and retained capture

Schema 16 distinguishes service and account operations with shared durable ordering and independent prior state. The verified-account worker invokes only fixed native repository moderation through the pinned node. Protected account leases retain generation, operation, DID, phase and native reference. Only platform-owned takedowns can be changed. Resume never changes user deactivation. The public API remains read-only for moderation; the browser reports account and service state separately. Monitoring and management recovery check confirmed native account fences while running and fail closed on uncertainty.

Actual Bob qualification completed at 2026-10-01T09:55:14Z. Native takedown denied ordinary login, retained-session writes and public repository serving. Operator export still verified signed data and media. Alice remained available. Customer escalation and foreign-owner reads returned 404. An audited service hold fenced every writer for a 435,712-byte cold archive, including both leases and protected runtime metadata; authenticated encryption/decryption produced an identical digest. Service resume retained the account takedown. Separate account resume restored the original confirmed account and custody, and an older operation failed. Report: `.local/account-lifecycle/b7554d52-eda4-4260-9706-c2279451f773/report.json`.

The first drill failed before applying native moderation because Node interpreted the fixed helper argument `inspect` as its debugger command. The replacement `native_status` action passed an actual invocation and the corrected drill. The failed operation remains audited. No native data was deleted.

Subsequent encrypted recovery included both running leases. It completed at 2026-10-01T09:56:51Z with 17 files, five SQLite databases, five signed records and one referenced blob. Fresh signed writes and recovered PLC signing passed; original and restored writers remained fenced. Report: `.local/managed-recovery/cb7c09eb-8b93-4ac1-b7a8-6ebbf0c815d3/report.json`.

The full local smoke gate passed frontend build, ten Node tests, Go build/vet and race/integration tests. Customer export, held restoration, independent backup custody, notifications, broader operator authorization and live qualification remain open for #17. Actual user deactivation and new responsive UI behavior remain unqualified. See [local account lifecycle](../local-account-lifecycle.md).

Managed recovery [PR #52](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/52) passed [CI run 11406](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11406) at `9d6fe22`. Release rehearsal [PR #53](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/53) passed [CI run 11407](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11407) at `ea4ee28`. Service lifecycle [PR #54](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/54) passed [CI run 11410](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11410) at `04831a0`.

### Current management inventory and copied-authentication revocation

Management recovery hold now revokes copied browser sessions and unfinished login attempts in the same transaction as the hold. The loopback disposable-database restriction remains unchanged. Source ownership, credentials and sessions are preserved. Regression coverage verifies transient revocation while retaining owners and the recovery hold.

The current qualification discovers every public management/River table and verifies complete private row digests, counts and sequence positions before installing the restored hold. Source API, worker and monitor were paused only for this project to keep its inventory stable. The latest schema 16 restore completed at 2026-10-01T10:09:34Z. Twenty-six inventoried tables matched, including two owners, four organizations, four deployments, two placements, four enrollment operations, two usage observations, seven service/account operations and twenty River jobs. Forty copied management sessions were revoked; worker startup refused the held copy. Original tables and sequences remained unchanged.

Host reconciliation matched both managed tenants and their exact lifecycle fences, with explicit acknowledgement of the earlier manual local namespace. A real River lost-checkpoint replay completed for Bob at 2026-10-01T10:09:45Z. Source management rows/sequences, native signed data/media, PDS/ingress/guard container IDs and protected credential/TLS/admission/lifecycle files remained unchanged. Reports: `.local/management-recovery/5d7bda80-6de6-4669-93c6-6f4599a10ffa/report.json` and `replay-report.json`. Earlier schema 16 copies remain retained. Source services resumed afterward.

The full local smoke gate passed. Actual recovery/replay and script syntax checks passed. Independent backup storage, key/certificate-loss recovery, retention, scheduled capture and live qualification remain open for #13. See [local recovery](../local-recovery.md).

Native account lifecycle [PR #55](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/55) passed [CI run 11413](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11413) at `174f45d`.

### Scoped browser-approved customer CLI

Schema 17 adds ten-minute device grants, browser approval with code/permission review, and revocable 24-hour CLI credentials. The Go client provides login, organization listing, deploy/list/status, logout, offline help/version and bash/zsh/fish completions. Deployment requests are persisted before submission, and Ctrl+C leaves server work running. Credential files are private; tokens never appear in URLs, command arguments or terminal output. The fixed scope excludes browser sessions, organization mutation, native invitations and host administration.

Actual CLI qualification `afc742d3-fc37-40ce-b1e9-96888cc15c6d` completed at 2026-10-01T10:33:50Z. Alice/Bob ownership and revocation fences passed; interrupt/retry preserved one simulated deployment, which completed after the worker resumed. Existing Bob native status was readable only by its owner. This was a simulated new deployment because all three local native slots remain occupied.

Schema 17 recovery `236cbd45-e887-401d-b10a-9709f92279a1` matched all 29 management/River tables and sequence positions. The restored hold revoked one copied CLI credential, six grants and 47 browser sessions. Lost-checkpoint replay preserved all source rows/sequences, native signed data/media, container identities and protected files. The source credential stayed valid during recovery, and final all-device revocation rejected both it and an approved unconsumed grant. Source API/worker/monitor resumed.

Full smoke passed, including Go vet, race-enabled clean-database integration tests and frontend build. Five platform binaries/checksums were prepared locally. Actual macOS ARM64 authorization and Linux ARM64 offline runtime checks passed; other platforms are cross-compile evidence only. The first macOS launch exceeded the cold-start target; warmed median was about 6 ms. Browser snapshots failed, so responsive approval/revocation UI verification remains open. Nothing was published. [Detailed evidence and remaining gates](../local-customer-cli.md). Issue #27 and live qualification remain open.

### Explicit local pilot entitlements and native allocation limits

Schema 18 records immutable operator grant/revoke operations with identity, reason, stable key and expiry. Two local qualification profiles allow one or three native intents; there are no configured prices or paid subscriptions. The native deployment transaction locks the organization, resolves existing-key replay, then checks active entitlement and retained allocation before inserting any new operation or job. Grant changes use the same lock. Expiry/revocation/downgrade blocks growth without changing existing runtime, account authority, retained data or accepted work.

Full smoke passed, including eight concurrent native allocations yielding exactly one accepted operation under a one-instance allowance. Actual local drill `cdc543e2-d2cb-4f36-bd9f-5e02df2f46fe` proved unconfigured/over-limit denial, downgrade retention, foreign-owner/customer-mutation denial and Bob's stable deployment replay while revoked. Source deployment/event/deploy-job counts and both tenants' containers/credential/TLS/admission digests stayed unchanged. Explicit audited 24-hour qualification grants remain for Alice and Bob. No native instance or payment was created; all three local node slots remain occupied. Source API/worker/monitor resumed on schema 18.

Schema 18 recovery `cc1378d3-81af-4b95-94e5-88d2ace176cc` matched all 30 management/River tables and sequence positions, including eight grant records. Held-copy authentication revocation and lost-checkpoint replay passed against unchanged source data/custody. Latest grant states/expiries and derived allocation counts matched both organizations after restore. Source management services resumed.

[Details and paid-integration gates](../local-pilot-entitlements.md). Stripe sandbox checkout/portal, verified webhook reconciliation, published prices/grace policy, storage/account enforcement, release/export and live qualification remain open. Issues #18/#19 are not closed. CI for CLI PR #57 passed as run 11418; the exact preceding recovery commit passed workflow retry 11417 after its earlier task stopped without a retained log.

### Signed local billing inbox and current-state reconciliation

Schema 19 adds immutable fixture ownership bindings, a signed event inbox and subscription observations. Event receipt and River work commit together. The worker serializes authority reads by organization and records current subscription state rather than trusting delivery order. Observations do not grant paid entitlement or alter pilot grants/native resources. Explicit loopback fixture mode is the only enabled adapter; ordinary startup leaves billing disabled.

The pinned Stripe Go SDK is v87.0.0 with API version `2026-09-30.endive`. Raw-body signatures are verified before parsing, with bounded delivery sizes, timestamp checks and live/account/context rejection. Duplicate checks retain immutable reconciliation references while accepting changed delivery metadata. The first raw-body digest is retained; raw payment/customer content is not stored. Full smoke passed build/vet/race tests, transactional/concurrent reconciliation coverage, migrations, frontend build and Node checks.

Actual drill `afc197b2-f48c-4328-a270-66ab42091cef` completed at 2026-10-01T11:38:32Z. Four events reconciled, duplicates produced one job, changed identity failed, and an older paid notification preserved current canceled state. Native resources/custody, deployment rows/jobs and eight pilot grant records remained unchanged. Two drills left one binding, eight inbox events and one observation. Private report: `.local/billing-fixture/qualification/afc197b2-f48c-4328-a270-66ab42091cef/report.json`.

No Stripe sandbox resources or charges were created. Checkout/portal, paid permissions, grace/risk policy, periodic reconciliation, tax, notification and live billing gates remain open for #18. See [local billing inbox](../local-billing-inbox.md).

Fresh schema 19 management recovery `e10ebba7-f473-40b8-95cb-ad7e9b51b458` matched all 33 management/River tables and sequences, including eight billing events and 29 River jobs. The restored hold revoked copied authentication and refused worker startup. Exact host reconciliation and lost-checkpoint replay preserved source rows/sequences, populated signed data/media and protected host state. Reports remain in `.local/management-recovery/e10ebba7-f473-40b8-95cb-ad7e9b51b458/`. Source management writers were paused across capture/replay and resumed with billing disabled afterward.

Billing inbox [PR #59](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/59) passed [CI run 11424](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11424) at `9841a92`.

### Bounded tenant CPU pressure and cgroup OOM

The local resource drill checks actual one-CPU, 768 MiB RAM, additional 768 MiB swap and 128-PID cgroup limits before applying fixed probes to Bob. Two worker threads run for five seconds. A finite memory probe increases its own OOM selection score and touches bounded buffers. Alice uses one native login and repeated authenticated session reads. Before and after, operator export verifies both signed repositories and every referenced blob; container IDs and protected host files must match.

Final drill `2834130d-97ff-4b89-b80b-02a664f71ca0` completed at 2026-10-01T11:56:04Z. The CPU throttle counter increased by 50. Memory-limit events increased by 2,213 and the cgroup recorded one OOM kill; the probe exited 137. All 67 neighbor authenticated reads passed, with a worst observed response of 103 milliseconds. Both tenants retained signed data/media, containers and protected custody/lifecycle/admission state. No native records or management lifecycle intents changed.

An initial cgroup helper failed before pressure due to a generated newline and was corrected. The first successful probe used repeated logins; an immediate repeat hit native rate protection before pressure. The final version reuses one session and passed after the login window reset. Private reports remain under `.local/resource-pressure/`, including the earlier successful drill `2558830d-7352-4f17-b518-df62aca256ff`. Full smoke and final script syntax checks passed.

This is local fixed-probe evidence for #16. PID exhaustion, storage/account limits, sustained request load, bandwidth limits, hostile-tenant security and independent host failure boundaries remain open. See [resource pressure](../local-resource-pressure.md).

Resource pressure [PR #60](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/60) passed [CI run 11425](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11425) at `76ad7c4`.

### Fresh restoration preserves native account takedown

Managed recovery now supports a fixed held-account mode. It requires the confirmed account fence, uses native operator export to verify signed repositories/media, preserves public login/repository denial and skips ordinary writes while held. Source protected metadata must match after restoration. Recovery fixtures choose unused fixed local `/24` subnets without deleting retained networks. Compose dollar escaping and actual container command/environment checks preserve the admission guard and runtime values. The outer qualifier records audited account hold/resume intent and waits for Docker health before source account resume.

The final held drill completed at 2026-10-01T12:04:07Z. Inner recovery `74359c17-2560-4ed3-83cb-0cdf698cd713` captured 17 files, five SQLite databases and 625,413 bytes. Fresh storage retained five signed records, one referenced blob, source TLS/endpoint and protected custody/fences. Restored public login and repository serving were denied; operator export and cloned-directory PLC signing passed. Restored writers stopped before the source resumed. Audited account generation 19 suspended the source, and a separate generation 20 action restored native login with original data/media. Service generation 6 stayed running. Outer report: `.local/held-recovery/f64b1f33-bb0d-48c8-ae54-f3b5b2d966d7/report.json`. Its 5.974-second inner measurement describes a tiny local dataset only.

Earlier attempts exposed exhausted default Docker address pools and Compose interpolation of literal JavaScript. Configuration checks rejected incorrect escaping before fresh writers started. All fresh/staging volumes and previous networks remain retained. The first failed restoration's account resume also failed before Docker health settled; an explicit new audited action recovered it, and the helper now waits before submitting resume. Failed audit records were preserved. Source signed data/media remained intact.

Customer export, service-held restoration, user-deactivated accounts, independent custody, key/certificate loss and live recovery remain open for #12/#17. See [held account recovery](../local-held-recovery.md).

The ordinary running-account regression also passed at 2026-10-01T12:06:07Z. Recovery `54e5deb1-2208-4d09-b50a-307bd626e83f` captured 17 files, five SQLite databases, six source signed records and one referenced blob. Fresh native writes/signatures and cloned-directory PLC signing passed with source protected metadata retained. This drill added its usual source marker; Bob now has six signed records.

Afterward management recovery `7ec693ba-f99f-4acd-aadc-778ea72c087f` matched all 33 tables and sequences, including 20 lifecycle operations, 40 lifecycle audit events, eight billing events and 42 River jobs. Copied authentication was revoked and held worker startup refused. Exact host reconciliation and lost-checkpoint replay preserved current generation-20 account custody, source management rows/sequences and native signed data/media. Reports remain under `.local/management-recovery/7ec693ba-f99f-4acd-aadc-778ea72c087f/`. Full smoke, subnet/escaping tests and script syntax checks passed. Management services resumed with billing disabled.

Held recovery [PR #61](https://git.subcult.tv/PatrickFanella/pds-ass/pulls/61) passed [CI run 11430](https://git.subcult.tv/PatrickFanella/pds-ass/actions/runs/11430) at `06fde4e`.

### Explicit operator support authority and audited inspection

Schema 20 records support inspections before disclosing selected management evidence. A private startup policy binds exact verified OIDC issuer/subject pairs to inspect-only or lifecycle-action authority, independently of customer ownership. Email/display names cannot grant access. The policy is local-only and disabled by default; production rejects this qualification configuration. CLI credentials cannot use the operator namespace. Every action checks browser session, operator identity, origin and CSRF.

Operators inspect exact organization/instance references and bounded lifecycle history. Responses exclude native credentials, private records, invoice content, emails, request keys and raw host errors. Fixed service/account actions use the existing atomic lifecycle/audit/River path and restricted agent. Actor attribution uses the authorized management owner reference. Customers can read only their own lifecycle activity, with actor identities redacted. The browser support section retains an owner-specific request before submission and requires review before a retry or a new action.

The local gate passed frontend build, Node checks and Go vet/race/integration tests. Coverage includes exact identity/issuer matching, private policy restrictions, inspect-only denial, fixed action replay/attribution, audit persistence failure, redaction, owner isolation and history pagination. Optional golangci-lint remains unavailable.

Actual drill `8b27f0b9-3199-44d6-aa24-58c6c57c2e15` completed at 2026-10-01T12:23:34Z. Alice's explicit operator identity inspected Bob through the management API; Bob had no operator rights. Missing CSRF failed, foreign customer activity returned 404, and customer history hid operator identities. Generation 21 applied native account takedown, duplicate submission retained one operation, and generation 22 separately resumed the account. Alice remained available. Both signed repositories, referenced media and protected credential/TLS/admission state remained unchanged. Bob retained six signed records and one referenced blob. Report: `.local/support-workspace/8b27f0b9-3199-44d6-aa24-58c6c57c2e15/report.json`.

Collaborative preview status/open succeeded, but snapshots still failed twice on client `preview-2159aa7e21ca3525611142bfa246cff8`. Responsive layout, keyboard/screen-reader use and browser persistence remain unqualified. Dedicated operator identity/MFA/revocation policy, notifications, deployment retry and live support gates remain open for #22. See [operator workspace](../local-operator-workspace.md).

Current schema 20 management recovery `0c044277-e072-4e7a-b241-cfb3dfb6ac85` matched all 34 tables and sequence positions, including five inspection audits, 22 lifecycle operations, 44 lifecycle events, eight billing events and 44 River jobs. Copied authentication was revoked and held worker startup refused. Exact host reconciliation/replay preserved source rows/sequences, native signed data/media and generation-22 account custody. Source API/worker/monitor resumed with both fixture operator policy and billing disabled. Reports remain under `.local/management-recovery/0c044277-e072-4e7a-b241-cfb3dfb6ac85/`.

## Portable customer exports, schema 21

Owner-authorized stable export requests and River jobs now commit together. A restricted mTLS agent captures the sole expected native actor, verifies repository signatures and media hashes, and returns portable content only. The separate encrypted local artifact store preserves original capture across lost-checkpoint replay. Downloads verify the full archive and its management checkpoint before responding. Private native settings and recovery keys remain outside the customer bundle. See [local customer exports](../local-customer-exports.md) and [the export contract](../customer-export-design.md).

Actual drill `dd82134d-ef81-4c17-b636-940c86cc1aea` completed at 2026-10-01T13:15:28Z. Normal export `4b4ccb0e-377c-4db0-a775-b5e2a26f8ddc` and held export `b5262db8-6bf3-4333-8917-aa736a8866af` each downloaded 6,656 bytes containing six signed records and one verified media blob. Independent offline verification checked signatures, CIDs, manifest hashes and portable-only members. Stable retries resolved one request; foreign-owner list/progress/download/capture, missing CSRF and caller-supplied DID were rejected. Generation 23 held public login while export succeeded; a separate audited generation-24 action resumed the account. Native signed data/media, container identities and protected credential/TLS/admission files remained unchanged.

Artifact recovery `9580a614-5ff6-4eca-af5f-22ca99eaa26e` restored fresh copies from an authenticated backup with keys separate from its bundle. Wrong key, tampering and wrong scope failed. Recovered signatures/media and original archive hashes matched; source artifacts/key remained unchanged. Copies are all on this workstation, so independent storage remains unqualified.

The full local smoke gate passes Node checks, frontend build and Go vet/race/PostgreSQL tests. Tests cover atomic queue rollback, eight concurrent requests yielding three retained operations, owner isolation, expired download denial without deletion, artifact tampering, immutable replay, fixed helper framing and paused-service/recovery holds. Optional golangci-lint remains unavailable. The browser download workflow is implemented; responsive/accessibility/persistence checks remain unqualified after the existing T3 preview snapshot failure.

PR #62's exact commit `f5113d9` passed remote CI run 11433. Export work continues on top of that branch. Independent custody/retention, larger datasets, live downloads, cancellation and actual migration remain open.

Schema 21 management recovery `c399a80b-2a9d-4ff6-ba6b-96112cb51a2c` matched all 36 management/River tables and sequences, including two export operations, four export events, 24 lifecycle operations, 48 lifecycle events and 48 River jobs. Held-copy authentication was revoked and worker startup refused. Exact host reconciliation and lost-checkpoint replay preserved source rows/sequences, native signed data/media and generation-24 account custody. Reports remain in `.local/management-recovery/c399a80b-2a9d-4ff6-ba6b-96112cb51a2c/`. The source management writers were paused across capture/replay.

Source API, worker and monitor resumed on schema 21 with fixture billing and operator policy disabled. The native agent retains the existing certificates/state and three-slot ceiling. No source container, volume or retained recovery copy was removed.

## Private preferences in portable exports

Version 2 archives add a bounded `preferences.json` for private `app.bsky` settings. The fixed helper selects the expected actor's preference name/value columns through a read-only SQLite transaction, checks counts/bytes before retrieval and validates namespace/type agreement. It reads no authentication settings or key files. Existing version 1 snapshots remain immutable on replay. Unsupported namespaces, excessive content and invalid wrappers are rejected. See [export and exit](../export-and-exit.md) for supported state and migration preparation.

Actual drill `d4341ccf-c73b-4ffa-9cc3-581b6688218f` completed at 2026-10-01T13:29:46Z. Alice downloaded a 7,168-byte archive with four signed records, one blob and one private preference. Bob downloaded a 7,680-byte archive with six signed records, one blob and one private preference while generation 25 held public login. Exact preference hashes, signatures, media and owner isolation passed independent verification. Generation 26 separately resumed Bob. The drill seeded and then removed one exact marked synthetic preference in each initially empty local actor table because these fixtures have no configured AppView preference API. Prior preference values, source signed data/media and custody were retained; public preference API use remains unqualified.

An actual owner retry downloaded the original version 1 snapshot with its original hash after the format change. Fresh artifact recovery `a62a104e-2b5f-481e-954a-b8c8b8cb7bf8` authenticated both version 2 snapshots against management byte/hash checkpoints, restored separate copies, verified preference/media/signature integrity and rejected tampering, wrong keys and wrong scope. Version 1 custody recovery also passed after the verifier update. Source encrypted artifacts and their key remained unchanged.

The full smoke gate passed, including preference bounds, namespace restrictions, private encryption and version compatibility. PR #63 commit `db5d34d` passed remote CI run 11436. Browser rendering/accessibility/persistence, independent storage, cancellation/retention, larger exports and actual migration remain open.

Fresh management copy `bd6efe22-13de-44d8-8224-1f0642448ccd` matched all 36 schema 21 tables and sequences, including four export operations, eight export events, 26 lifecycle operations, 52 lifecycle events and 52 River jobs. Copied authentication was revoked and held worker startup refused. Exact host reconciliation/replay preserved source rows/sequences, native signed data/media and generation-26 account custody. Reports remain under `.local/management-recovery/bd6efe22-13de-44d8-8224-1f0642448ccd/`.

Source management services resumed with fixture billing/operator authority disabled. Both account services are running; marked preference values were removed through exact-match cleanup. The export allowance now retains three Bob requests and one Alice request. Existing snapshots and all recovery copies remain retained.

## Dated local hosting exit, schema 22

Owner scheduling now binds the saved export, reviewed policy, original service-end/retention dates, audit and two River jobs atomically. Exit-owned service stop/resume uses the existing restricted-agent lifecycle path. Reversal preserves independent account moderation and newer operator holds. Retention schedules review without deletion authority. Production rejects the private rehearsal policy. See [local hosting exit](../local-hosting-exit.md) for configuration, exact drills, failure evidence and remaining gates.

The normal 14:05:44Z drill verified stop, unchanged download hash, reversal, owner isolation and 144 successful neighboring account reads. The complete held-account repeat at 14:20:33Z preserved the independent account hold after owner service reversal; a separate operator action restored login. Reviewed-policy binding passed at 14:33:00Z. After disabling the fixture policy, committed schedule/reversal replay and new-schedule refusal passed at 14:56:08Z. All seven accepted source exits are reversed; native signed records/media, containers and protected custody remain intact.

Fresh recovery `cccc7a4b-7394-41eb-8424-e30d0817b654` matched all 39 schema-22 tables and sequences, including seven exits, 23 exit audits, seven reversals, 36 lifecycle operations, 72 lifecycle audits and 79 River jobs. Copied authentication was revoked and held worker startup refused. Dedicated-queue reconciliation/replay preserved every copied default-queue row, including an explicitly available retention job, and source/native custody. Reports remain under `.local/management-recovery/cccc7a4b-7394-41eb-8424-e30d0817b654/`.

The full smoke gate passed. Source API, worker and monitor resumed with new exit scheduling, fixture billing and operator authority disabled. Both native services run; no retained containers, volumes or recovery copies were removed. PR #64 commit `2c8d9a1` passed remote CI run 11439. Browser rendering/accessibility/persistence, independent storage, paid cancellation, notifications, confirmed deletion, migration and live qualification remain open.

## Owner hosting history

The read-only owner journey joins existing deployment, enrollment, domain, service/account, export and exit audits into a bounded timeline with fixed next steps. Current organization ownership and private operation owner bindings remain enforced. Cursor pagination preserves full timestamp/category/integer ties and binds the instance. It returns no raw messages, domain values, actors, request keys or native private fields. It creates no host work or outbound notification. See [owner hosting history](../local-owner-journey.md).

Full smoke passed. Actual drill `22f25f60-15f3-4b06-bd62-f887d5c8068e` completed at 15:06:40Z: Alice's 60 events and Bob's 69 events each traversed two pages without loss or repetition. All seven event categories appeared. Read replay matched; foreign owners, cross-instance cursors, malformed queries and CLI scope were denied. Audit counts, source signed records/media, containers and protected custody remained unchanged.

The collaborative preview reports availability at 375 by 812, but snapshot still fails on the existing client. Browser responsiveness, accessibility and persistence remain unqualified. PR #65 commit `1d9ea48` passed remote CI run 11444. Source API runs with exit scheduling, fixture billing and operator authority disabled. Email notification and live qualification remain open.

## Identity-bound local organization collaboration, schema 23

A private local opt-in enables management invitations, scoped administrator/viewer membership, effective-change audits, immediate revocation and separately accepted empty-organization handover. Invitation acceptance binds exact verified issuer/subject identity and expires after 24 hours. Shared server status uses a small metadata projection; existing owner APIs, native account authority and CLI permissions are unchanged. See [local management collaboration](../local-organization-access.md) for role limits and remaining work.

Full smoke passed. Actual drill `38d7984c-e379-4fad-80b8-b92f9775860a` completed at 15:26:53Z: viewer status succeeded while private exports and role escalation were denied; missing CSRF failed; revocation was immediate and accepted invitation replay did not regrant access. A new empty organization exercised administrator rename, separate ownership offer/acceptance, one acceptance audit and former-owner removal. Source native signed data/media, containers and protected custody were unchanged. The source hosting membership remains revoked.

Fresh recovery `cbc79c44-2096-4503-b138-90e2aa1af9ca` matched all 43 schema-23 tables and sequences, including two invitations, two retained membership rows, one transfer and eight access audits. Copied authentication was revoked and held worker startup refused. Host reconciliation and dedicated-queue replay preserved copied default work, source rows/sequences and native custody. Source API/worker/monitor resumed with collaboration, exit scheduling, fixture billing and operator authority disabled; an authenticated request confirmed the collaboration route is disabled.

PR #66 commit `faf7ef1` passed remote CI run 11447. Browser collaboration controls, delegated administrator hosting/domain work, full list pagination, populated handover and live qualification remain open. Both native services run; no container, volume or recovery copy was removed.

## Browser collaboration, reviewed persistence and recovery

The collaboration browser controls now stage a typed owner-scoped request before mutation, show the exact identity/role/organization, require fresh review after reload or failure and preserve stable invitation/offer requests. Viewer, administrator and owner controls reflect their qualified API scope. Separate recipient acceptance remains required for empty-organization ownership. See [browser collaboration](../local-collaboration-workspace.md).

Native T3 DOM evaluation/interactions work despite snapshot capture failure. The actual browser drill completed at 16:05:05Z: invitation persistence without automatic submission, keyboard Tab/Space/Enter confirmation, focused review/receipt, owner-separated retained state across sign-out, viewer controls, administrator rename/invitation boundary, ownership offer/acceptance and former-owner denial passed. A local API outage retained the action and reset acknowledgement; explicit retry after recovery recorded one rename. Both native signed repositories/media, containers and protected custody remained unchanged.

Viewer and owner review/form layouts had zero document overflow/offscreen collaboration elements at 320×568, 375×812, 812×375, 768×1024 and 1440×900. Visible controls had labels. Snapshot appearance, assistive screen-reader testing and broader failure journeys remain unqualified. Full smoke and final frontend build passed. PR #67 commit `1450971` and merged main `081ea55` passed remote CI runs 11485 and 11486.

Default restart hid collaboration controls while the ordinary owner workspace stayed available. Empty browser test organization `a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c` and its eight effective access audits remain retained under Bob's ownership. No native host operation, container/volume deletion or external invitation message occurred.

Post-browser recovery `b3a05f17-fa9f-4599-85af-4b5d39dab6de` matched all 43 schema-23 tables and sequences, including three invitations, three retained membership rows, two transfers, 16 access audits and 79 River jobs. Copied authentication was revoked and held worker startup refused. Reconciliation/dedicated-queue replay preserved copied ordinary jobs, source management rows/sequences and native custody. Recovery copies remain retained.

Source API, worker and monitor resumed after this checkpoint with collaboration, exit scheduling, fixture billing and operator authority disabled. Native services remain running.

## Browser history focus and layouts

Actual native T3 DOM and keyboard checks traversed Bob's hosting history with 50 latest events and 19 older events. Loading the final page exposed a focus loss when the pager disappeared. The hosting and service/account history components now focus their announced page count after a successful load. The activity panel displayed 26 actions. Keyboard checks confirmed focus remained in the history workflow and the pager had a visible focus outline.

Expanded histories had no document overflow or offscreen history elements at 320×568, 375×812, 812×375, 768×1024 and 1440×900. The page had no duplicate IDs or unlabeled visible inputs. Private evidence remains in `.local/history-browser-widths.json`. Full smoke passed. These read-only browser checks create no management or host operation. Snapshot appearance and assistive screen-reader testing remain unqualified. See [owner hosting history](../local-owner-journey.md).

## Durable local deployment admission, schema 24

An operator can pause new native and simulated deployments without stopping servers, deleting storage or invalidating matching retained requests. Accepted intent transactions hold a shared policy lock through job commit; closing waits for them to finish. Effective policy changes and their audits commit together. Missing policy and recovery holds refuse new intent. Only the explicit local operator command can change admission. Production provisioning remains independently disabled. See [local deployment admission](../local-deployment-admission.md).

Full smoke passed. Actual drill `883cacc8-ba47-4123-b8fe-5566a631a34b` completed at 16:45:09Z: new requests were rejected, original native retry returned its existing operation, changed payload and foreign owner were denied, and status/history/export reads remained available. Repeated close produced no extra audit. Separate reopen restored the initial open policy. Two effective audits were added; deployment/event/job/lifecycle counts and both native signed repositories/media, containers and protected custody were unchanged.

Fresh recovery `3af12916-6b10-46c8-8beb-819e7df533db` matched all 45 schema-24 tables and sequences, including one admission policy, two admission audits, 79 River jobs and 36 lifecycle operations. Copied authentication was revoked and held worker startup refused. Reconciliation/dedicated-queue replay preserved copied default work, source rows/sequences and native custody. The source writers were paused across capture/replay; all copies remain retained.

Source API, worker and monitor resumed on schema 24 with admission open and collaboration, exit scheduling, fixture billing and operator authority disabled. Production signup/enrollment controls, independent infrastructure and signed-off release qualification remain open. PR #69 commit `cae90c8` passed remote CI run 11504.

## Paged organization access history

Owners can read every retained access audit through 50-event pages. Cursor scope and exact bigint IDs are preserved; each read-only page rechecks current ownership. Viewers, rename-only administrators and CLI credentials cannot read owner history. The browser cancels stale reads and focuses successful counts or failed-load errors. Access denial clears cached events. See [collaboration history](../local-collaboration-history.md).

Full smoke passed. Tests traverse 157 events beyond JavaScript integer precision over four pages and 117 events across digit boundaries. The actual read-only drill `3c20cfa0-58c0-475d-b6b8-f55d832366d5` completed at 16:59:52Z: Bob read eight retained events, read replay matched, former owner Alice and malformed/cross-organization cursors were denied, and audit/job/native custody remained unchanged.

Actual keyboard loading focused the page count, and five representative widths had no document/history overflow. Native T3 text and image snapshots now work; a 375×812 history screenshot was reviewed. At 17:04:12Z, disabling collaboration cleared cached events, focused the error and cleared busy state. Refresh then hid collaboration while ordinary owner controls stayed available. Private browser evidence remains under `.local/`. Larger-page browser navigation and assistive screen-reader checks remain unqualified.

The browser exposed lexicographic sorting of the displayed ID alias. The query now orders the qualified numeric column; a digit-boundary regression test covers it. A ready-API repeat recovered from an initial startup race without source mutation. Source API has resumed with collaboration and other fixture permissions disabled. The schema remains 24, so the prior 45-table checkpoint remains the current schema recovery evidence. No native container, volume or recovery copy was removed.

PR #70 commit `58f3295` passed remote CI run 11505. PR #71 commit `535f1e5` passed remote CI run 11510. Other collaboration list pagination and live milestone gates remain open.

## Paged permitted organizations

The browser can reach permitted organizations beyond the previous 100-row bound through 50-row pages. Exact timestamp/UUID cursors bind the current management identity; each query rechecks ownership and active membership. The old array response remains compatible. Loading replaces one bounded page, clears selected details, cancels stale reads and focuses successful counts or failed-load errors. No management action is confirmed by paging. See [permitted organization pages](../local-organization-pages.md).

Full smoke passed. Tests traverse 118 organizations over three pages with exact timestamp ties, then check role changes, revocation and ownership changes between pages. Actual read-only drill `0e232800-fd86-4e47-b75d-0c65a508785f` completed at 17:15:13Z: Alice read two organizations and Bob read four, repeats and compatible-route projections matched, malformed/cross-identity cursors and CLI scope were denied, and audit/job/native custody remained unchanged.

The actual browser keyboard reload focused Bob's four-row count. A temporary read-only browser fixture exercised 50 synthetic rows followed by one final row and kept focus after the pager disappeared. A delayed synthetic response was discarded after current-access refresh. All fixtures were removed and original fetch restored before reloading real data; no source organization was created. These qualify browser behavior only. Five widths had no document/collaboration overflow. At 17:21:06Z, feature disablement cleared the list and focused the error; refresh hid collaboration while ordinary owner controls remained available. Metadata remains in `.local/organization-pages-browser.json`.

T3 snapshots remain intermittent: history snapshots succeeded, while the list text snapshot failed. Assistive screen-reader, remaining list pagination and live milestone gates remain open. Source API is on schema 24 with collaboration and other fixture permissions disabled; admission is open. No native container, volume or recovery copy was removed. PR #72 commit `a2dd178` passed remote CI run 11515.

## Paged shared server status

Current owners and active members can read every shared server record through 50-row pages, with identity/organization/list-bound cursors and a fixed metadata projection. Every read rechecks current access. Browser paging cancels stale reads, clears denied records and focuses explicit results or errors. Existing bounded summaries and empty-organization transfer eligibility remain compatible. See [shared server pages](../local-shared-instance-pages.md).

Full smoke passed. Tests traverse 117 records across three pages, including timestamp ties, scoped cursors and access changes. Actual read-only drill `206a3a4b-2e0b-4aa5-a0e5-1621100e74e9` completed at 17:34:35Z: Alice read four records, Bob read one and the empty organization returned none. Compatible-route projections/repeats matched, malformed/cross-identity cursors and CLI scope were denied, and audits/jobs/native custody remained unchanged.

The actual browser loaded Bob's native record. A temporary read-only response fixture exercised 50 synthetic records then one final record with count focus after the pager disappeared. Original fetch was restored and real data reloaded; no source deployment was created. Five widths had no document/status overflow. At 17:40:03Z, feature disablement cleared records, focused the error with a visible outline and cleared busy state. Refresh hid collaboration while ordinary owner controls remained available. Private metadata remains in `.local/shared-instance-pages-browser.json`.

An interrupted local execution stopped dev processes; existing Docker resources remained healthy. Agent, issuer, API, worker, monitor and Vite resumed using existing state/certificates, and a fresh complete smoke gate passed. Source API is on schema 24 with fixture permissions disabled and admission open. Membership/invitation/offer pagination, assistive screen-reader and live milestone gates remain open. No native container, volume or recovery copy was removed.

PR #74 commit `9107763` passed remote CI run 11545. Merged main `99bbc71` through PR #73 passed run 11537.

## Paged owner membership and invitation receipts

Current owners can traverse retained memberships and invitation receipts through separate 50-row pages. Membership ordering uses immutable identity references; invitation ordering preserves timestamp/UUID ties. Read-time expiry creates no mutation. Active viewers/administrators, former owners and CLI credentials remain excluded. Browser loads clear failed reads and focus results/errors; changes still require the existing staged request and fresh review. See [membership and invitation receipts](../local-membership-receipts.md).

Full smoke passed. Tests traverse 117 rows of each kind over three pages and verify scope, current states and no expiry/audit mutation. Actual drill `4532585a-71ce-4bbf-aa4a-4c70c9a511cf` completed at 17:52:58Z: compatible projections/repeats matched and denied-access checks passed without changing audits/jobs/native custody. The qualifier now compares expiry instants after an initial timezone-string mismatch.

Actual browser receipts showed the retained revoked member without action controls and an accepted invitation; keyboard reload focused the count. Temporary read-only 50-to-one fixtures preserved focus for both final pages, then original fetch and real data were restored. Five sizes had no document/receipt overflow. At 17:55:53–54Z, default feature disablement cleared both lists, focused errors and cleared busy state. Refresh hid collaboration while ordinary owner controls stayed available. Metadata remains in `.local/membership-receipts-browser.json`; T3 text snapshot succeeded.

Source API is on schema 24 with fixture permissions disabled and admission open. Incoming-offer pagination, assistive screen-reader and live milestone gates remain open. No native host operation, container/volume deletion or recovery-copy removal occurred.

PR #75 commit `933f0f8` passed remote CI run 11550.

## Paged incoming invitations and ownership offers

Separate 50-offer pages remove the browser's 30-of-each limit while retaining the old response. Each page checks current recipient identity, expiry, offering ownership and required membership. Cursors bind identity/list kind. Acceptance still stages the existing persisted request with fresh acknowledgement and independent current eligibility checks. See [incoming offer pages](../local-incoming-offers.md).

Full smoke passed. Disposable PostgreSQL tests traverse 117 offers of each kind with tied timestamps, then remove expired/revoked/identity-mismatched or no-longer-authorized offers between pages. A UUID/text fixture parameter conflict was corrected. Actual read-only drill `2d5ec057-768a-4ec8-8b9c-258894dcd14d` completed at 18:02:03Z: both identities have zero pending offers, legacy projections/repeats and denied-access checks passed, and audits/jobs/native custody remained unchanged.

Actual browser keyboard reload focused the empty count. Temporary read-only 50-to-one responses preserved final-page focus; synthetic ownership review remained unchecked/disabled and paging reset acknowledgement. The retained request was cleared through the reviewed UI, zero mutation requests were sent, original fetch was restored and actual lists reloaded. A delayed synthetic response after refresh was discarded. Five sizes had no document/offer overflow. At 18:05:03–05Z, feature disablement cleared both lists, focused errors and cleared busy state. Refresh hid collaboration while ordinary owner controls stayed available. Metadata remains in `.local/incoming-offers-browser.json`.

Source API is on schema 24 with fixture permissions disabled and admission open. Snapshot capture remains intermittent. Assistive screen-reader, delegated hosting/domain work, populated handover and live gates remain open. No host operation, native container/volume deletion or recovery-copy removal occurred.

## Durable local first-account admission, schema 25

Operators can pause new first-account invitation and replacement/recovery requests separately from deployment admission. Shared policy locking runs through intent/event/job commit, effective changes and audits are atomic, and missing policy/recovery holds fail closed. Matching retained retries precede the policy check. Already-committed jobs, issued invitations and existing status/recovery paths remain available. The local operator command has no host access; production enrollment remains independently unconfigured. See [enrollment admission](../local-enrollment-admission.md).

Full smoke passed. PostgreSQL tests cover locking/concurrency/audit rollback, held/missing policy, retained/new/foreign/changed requests, already-admitted worker completion, owner-only policy reads and command target restrictions. Actual drill `f2ce6efd-bee9-4e5b-8483-91f5a8affd75` completed at 18:13:48Z: new invitation/recovery requests were refused, retained retry and reads passed, two audits recorded close/reopen, and enrollment/events/jobs/lifecycle/deployment policy/native custody were unchanged.

A separate actual browser pause at 18:14:38Z retained the verified account and enabled status control, with no document/account overflow at five representative sizes. Separate reopen left four effective audits while four enrollment operations, 14 events and 79 jobs were unchanged. Temporary read-only failed-account/closed-or-unavailable policy responses disabled new recovery with zero mutation requests, then original fetch and actual state were restored. A refreshed observed-route repeat passed at 18:20:16Z after an initial preview evaluation failed. Evidence remains in `.local/enrollment-admission-browser.json`.

Fresh recovery `693e74a0-5b6a-4518-9fb2-d71507cd82cf` matched all 47 schema-25 tables and sequences, including the enrollment policy/four audits and retained work. Copied authentication was revoked, held worker startup refused, and reconciliation/dedicated replay preserved copied default jobs, source rows/sequences and native custody. All recovery copies remain retained.

Source API, worker and monitor resumed on schema 25 with both admission policies open and fixture permissions disabled. Public signup limits, issued-invitation revocation, assistive screen-reader, independent infrastructure and release signoff remain open. No host operation or native container/volume/recovery-copy deletion occurred. PR #76's CI run 11553 is queued; it is not yet verified remotely.

## Local custom-address preflight

Owners can check DNS hostname syntax and registrable-domain separation from declared app/OAuth hosts and the management origin. The local route requires browser ownership/Origin/CSRF and rejects CLI authority. It saves no choice, reserves no domain and runs no DNS/certificate/host operation. Retained native hosting gets a migration/replacement warning with its original endpoint preserved. See [custom endpoint preflight](../local-custom-endpoint-preflight.md).

Full smoke passed. Actual read-only drill `9b6f6eca-5b11-4f31-ad25-c2ad0902ebfa` completed at 18:34:59Z. Both native organizations received warnings; an empty organization received planning-only guidance. Repeats, hostname/domain-conflict/body validation and denied-scope checks passed without changing management work, deployment endpoints or native custody.

Actual keyboard submission normalized an empty-org address and focused the result at 18:35:42Z. A sibling app-domain edit cleared the result and focused its rejection at 18:36:39Z. Existing native warning/link checks passed at 18:36:44Z. A temporary read-only unavailable response cleared the result, retained inputs and focused the error at 18:40:59Z. Original fetch and actual data were restored after the fixture. A repeat used the observed API path after the first attempt found no new ResourceTiming entry.

Phone checks exposed form overflow inside an otherwise bounded document. The grid/input minimum-width fix passed a fresh frontend build. Final checks at five representative widths found no document/plan overflow or out-of-panel controls. Metadata remains in `.local/endpoint-preflight-browser.json`; snapshot capture failed and assistive screen-reader work remains open.

Schema remains 25, with no preflight writes and the existing 47-table recovery checkpoint retained. Default fixture permissions stay disabled and both admission policies stay open. Persisted endpoint choice, ownership proof, routing/TLS, supported activation and migration remain open. No native container, volume or recovery copy was removed. PR #76 commit `06f1fd3` passed CI 11553; PR #77 commit `c0d0ed5` passed CI 11554.

## Durable unverified custom-address choice, schema 26

The local management API can save/read/cancel a reviewed endpoint intent with stable keys, one pending choice per organization and atomic save/cancel audits. A scoped seven-day TXT instruction is prepared without any DNS or host call. Unverified addresses have no global reservation. Pending intent blocks new default native deployment and organization handover; native endpoints and retained deployment retries stay intact. See [endpoint choice](../local-custom-endpoint-choice.md). Browser mutation controls and ownership verification remain next.

Full smoke passed. Tests cover stable/concurrent retries, changed/foreign/former requests, audit rollback, recovery holds, caps/name bounds, independent same-hostname proofs, strict HTTP/production scope and a native-submission race that commits exactly one competing intent under the organization lock. An initial test used the wrong transfer method name; correcting it restored the vet/test gate.

Actual drill `5f4427be-f0e0-40e7-9955-6cf3314e1c94` completed at 18:56:06Z with two sequential save/cancellations. The qualifier then held both choices pending concurrently and recovered lost responses only by exact request replay. Repeat `014cb4d9-9c0d-46b5-8be3-4a70548690a7` completed at 18:58:13Z: independent same-hostname proofs coexisted; native fallback, changed/foreign intent, missing-CSRF and CLI requests were denied. Both were cancelled. Four cancelled receipts/eight audits remain, with jobs, deployments/endpoints, enrollment, organization access audits, both admission policies and native custody unchanged.

Fresh recovery `24489b6d-dbb4-4315-9359-9ba2402428f1` completed at 18:58:36Z, matched all 49 schema-26 tables/sequences, revoked copied authentication and refused held worker startup. Reconciliation/dedicated replay preserved default jobs, source rows/sequences and native data/custody. Source writers stayed paused through capture/replay and resumed afterward with fixture permissions disabled and both admission policies open. Earlier copies remain retained.

PR #78 commit `2e0dfeb` passed remote CI 11557. Browser choice review/retry/cancel, ownership verification, globally exclusive verified claims, routing/TLS/activation and migration remain open. No native container, volume or recovery-copy deletion occurred.

## Browser review of custom-address intent

Owners can read pending choice/TXT instructions, stage save/cancellation after preflight, and review exact requests retained in owner/organization-scoped session storage. Reload or failure keeps acknowledgement unchecked and sends no mutation. Current-state refresh clears cached instructions/notice and fresh review is required before retry. See [browser endpoint workspace](../local-custom-endpoint-workspace.md).

Actual staging/reload at 19:05:19Z/19:05:44Z focused review and sent no mutation. Save at 19:06:58Z showed TXT instructions and hosting/handover pause, then survived reload. Separately reviewed keyboard cancellation completed at 19:09:14Z and retained the receipt. Five cancelled choices/ten audits and zero pending choices remain. Jobs, deployments/endpoints, enrollment, admission policies and native data/custody matched the baseline.

A read-only unavailable-response fixture at 19:10:32Z intercepted one attempt, preserved the exact request, cleared cached state and focused error/reset acknowledgement with zero source mutations. Original fetch was restored. Reload at 19:11:02Z retained its hash and sent no mutation; actual-state read and keyboard clearing passed at 19:12:18Z. Some preview mouse attempts failed; readiness/keyboard repeats passed. DOM opening recovered cancellation review after collapsed-panel clicks failed.

A long-address/TXT/synthetic-review fixture passed the 320×568 overflow/bounds check. T3 disconnected before the other widths and cache-denial check, and snapshot capture failed. The disconnect also prevented restoration of that last scoped read-only fixture and synthetic browser intent. It blocks endpoint mutations and cannot auto-submit; source checks afterward still showed zero pending choices and unchanged management/native work. Cleanup instructions and remaining checks are in the runbook. This is an open browser gate, not a completed five-width or screen-reader qualification.

Schema remains 26 with the retained 49-table checkpoint; the new browser receipt follows its snapshot. Source API/worker/monitor retain fixture permissions disabled and both admission policies open. No native container, volume or recovery-copy deletion occurred.

The final browser smoke gate passed. PR #79 commit `85879a8` passed remote CI run 11560. Remaining browser checks and cleanup stay open after the connection loss.

## Transient endpoint TXT observation

The local current/original owner can request a bounded read-only observation of a saved TXT challenge. No customer resolver or URL is accepted. Cancelled/expired/local-test choices send no query; public DNS answers are bounded and raw records/errors are omitted. Scope, proof identity, cancellation and expiry are rechecked after lookup. A match creates no saved state, audit, domain claim or activation authority. See [DNS observation](../local-endpoint-dns-observation.md).

Full smoke and focused race/coverage verification passed with 84.5% package statement coverage. Tests cover exact/duplicate/other scoped challenges, missing/oversized/error responses, deadline/parent cancellation and cancellation/expiry/ownership/proof changes during lookup. Strict empty JSON, browser Origin/CSRF, CLI and unchanged production boundaries passed.

Actual drill `72c2f4bb-7acc-4fdc-be2f-98362ee0ef3d` completed at 19:24:00Z. Local-test guidance and denial checks passed; observations left choice/audits unchanged. Separate cancellation retained the new receipt. Six cancelled choices/twelve audits and zero pending choices remain. Jobs, endpoints, admission policies and native data/custody matched the baseline. Positive public DNS matching remains injected-test evidence only.

Expiry and the reported check time now share one PostgreSQL statement timestamp. The final smoke/coverage gates passed, and the updated API reread the cancelled receipt at 19:26:27Z without authority. Schema remains 26; the earlier 49-table checkpoint predates later browser/DNS receipts. Source services retain fixture permissions disabled and both admission policies open. No native resource or recovery copy was removed.

PR #80 commit `397abfb` passed remote CI run 11561. Disconnected browser-fixture cleanup/remaining viewports/cache denial, observation UI, controlled public DNS, exclusive verified claims, routing/TLS/activation and migration remain open.

## Browser TXT checks and completed local address layout checks

Pending choices now offer a separate typed read-only DNS observation. Scope/choice changes or review abort reads and clear results; review immediately hides stale content. Denied access removes cached choice/TXT instructions, resets acknowledgement and focuses the parent error. Phone organization content uses the full row, and TXT fields stack. See [observation controls](../local-endpoint-observation-controls.md).

T3 keyboard fixture at 19:34:18Z focused a non-authoritative result. Unavailable response/review clearing passed at 19:35:08Z. Resize repeatedly disconnected T3, and its open tool explicitly reported no automation host. The fallback used pinned Playwright 1.63.0, existing Chrome and a fresh ignored profile. No browser download or app toolchain change occurred.

The first headless check exposed asynchronous result clearing during review; rendering now hides it immediately. A passing phone screenshot then showed cramped content despite no overflow. Moving content below the icon/badge and stacking TXT fields fixed it. Final drill `465060fb-8c65-49a5-ba59-b3617fcb8d6c` completed at 19:45:15Z on Chrome 154.0.8037.92. Keyboard/failure/review/reload and DNS/current-choice denial checks passed at all five representative widths without overflow or out-of-panel controls. Phone panels are 230/285 pixels wide. The 375-pixel screenshot was inspected. Four DNS response fixtures ran with zero source mutation attempts; routes were removed, actual null choice reloaded and context closed.

T3 reconnected and shared original fetch/actual null state were restored before clearing the synthetic request at 19:46:38Z. Shared fixture cleanup is complete. An earlier readiness check ran before refreshed state rendered; the actual-null/readiness repeat passed before clearing. Private T3 metadata and isolated browser reports remain under `.local/`.

Source checks preserved six cancelled choices/twelve audits, zero pending choices, jobs/endpoints/admission policy and native data/custody. Schema remains 26, fixture permissions disabled and admission open. No native container, volume or recovery-copy deletion occurred. PR #81 commit `5efbba6` passed remote CI 11562. Controlled public DNS, exclusive verified claims, routing/TLS/activation, assistive screen-reader behavior and migration remain open.

The final full smoke gate passed after the phone/grid style change. All local address viewport/cache-denial checks and shared-fixture cleanup are now qualified; controlled public DNS and activation remain open.

## Read-only outbound migration inventory

Existing owned, unexpired exports now have a local browser-only inventory preflight. It authenticates the sealed archive/member hashes and management checkpoint, then rechecks scope, expiry and recovery status. It reports captured totals and version 1/2 preference coverage, with signature rechecking, transfer and identity switching explicitly unavailable. See [migration preflight](../local-migration-preflight.md).

Full smoke passed. Tests reject ownership/expiry/checkpoint/hold changes during artifact reading, cancelled contexts, tampering, missing custody, foreign owners, CLI and production use. Archive content and private preferences remain undisclosed; export/event/job rows stay unchanged.

Actual read-only drill `48734021-122b-4944-9b22-f4247020b9e0` completed at 20:02:34Z against all four retained archives. Both versions, exact repeats and scope denials passed. Independent offline verification confirmed captured signatures/media and inventory totals. Source management work, sealed artifacts and both native inventories/custody matched the baseline. Local sign-ins intentionally changed authentication activity. No new export, expiry reset, allowance reset, target allocation, import or identity switch occurred.

Schema remains 26, fixture permissions disabled and admissions open. Existing native containers, volumes and recovery copies remain retained. Current source authority/recovery, designated targets, resumable imports, post-switch checks and external application state remain open under issue #29.

## Owner migration inventory controls

Available exports now offer a separate bounded read-only inventory control. Exact checkpoint/scope, fixed non-execution flags, totals, archive version and remaining gates must match. Errors discard results and focus messages; scope/list denial removes cached inventory and download links. Reload retains no result and starts no inventory read. Capture-request errors remain separate from background list errors. See [migration workspace](../local-migration-workspace.md).

Actual version 2 keyboard inspection focused its result at 20:06:40Z; version 1 missing-preference guidance passed at 20:10:17Z. The first 320-pixel check exposed inherited flex layout in export rows. Scoped stacked rows/details and shrinkable deployment content fixed it. All five representative widths passed at 20:08:01Z–20:08:13Z, without overflow or out-of-panel controls. T3 snapshots repeatedly failed, so visual/screen-reader checks remain open.

Unavailable/invalid-authority fixtures cleared stale results and focused errors. Consistent scope/list denial removed inventory/download links and focused the parent error at 20:09:37Z. Three fixture reads and zero mutations occurred. Original fetch was restored at 20:09:49Z before actual reload; reload had no fixture or automatically loaded results. Source management/native work and custody matched the baseline. No source export, identity, hosting resource or recovery copy changed.

PR #82 and #83 remote CI runs 11580/11583 remain queued as of 20:09Z. Local API/inventory gates passed; this does not imply remote CI completion, visual qualification or migration completion.

The final full smoke gate passed after list and capture error states were separated. No actionable findings remain in the local inventory-control review. Remote CI, snapshot/visual qualification and live migration gates remain open.

## Export download authority recheck

Downloads now share the inventory disclosure guard. After authenticating the retained archive/checkpoint, the guard reads current/original scope, expiry and recovery status together before returning any bytes or inventory. Changed checkpoints and cancelled requests also return no content.

The pre-fix download body, with only an injected artifact-reader seam, disclosed stale bytes in all five authority-change regression cases. The shared guard passes both download and inventory cases. An initial expiry fixture only changed expiry and hit the exact 24-hour database constraint; its earlier denial did not exercise post-read expiry. The corrected fixture changes capture/expiry consistently and fails on any mutation error. Disposable test databases contain all regression mutations. Source archive dates remain unchanged.

Final full smoke passed. Repeat actual read-only drill `f4900482-269c-4f15-b50d-0c17a36fbd5e` completed at 20:18:04Z against all four retained version 1/2 archives. Owned downloads/inventory, scope denials and independent offline signatures/media passed. Source work, sealed artifacts and native custody matched the baseline. No source expiry reset, new capture, target allocation or identity mutation occurred. Schema remains 26; retained native resources and recovery copies remain untouched.

PR #82 commit `b0cb80b` and #83 commit `23097f5` passed remote CI runs 11580/11583. PR #84 run 11584 is in progress. No actionable findings remain in the disclosure-guard review after the corrected expiry regression and final gate.

## Repeated offline CLI platform checks

Fresh five-target binaries and checksum metadata passed the optional local runtime qualifier. Drill `f5740f1d-b4b4-40b7-b6e8-fd48983a66aa` completed at 20:22:34Z on native macOS ARM64, macOS AMD64 through Rosetta and isolated Linux ARM64. Each passed version, root/deploy help, three shell completions and noninteractive unknown-command/missing-sign-in exits with separate output streams. The first script attempt expected `sign in` while the actual diagnostic says `sign-in`; correcting the assertion allowed the unchanged binary to pass.

All five checksums matched before/after, private configurations remained empty, and existing containers/volumes matched the baseline. No account authorization, API request, new image, installation or publication occurred. Translated startup took 1,879 ms before later 14–17 ms cases; cold-start, Intel hardware, Linux AMD64, Windows/ACL, signing and live onboarding gates remain open. See [CLI distribution evidence](../local-customer-cli.md).

PR #84 commit `5217351` passed CI 11584. PR #85 commit `c102822` passed CI 11585. The final CLI runtime full smoke gate passed. Completion checks validate generated text only; shell integration remains unqualified.

## Translated CLI local device authorization

Drill `cba280d7-3676-4139-a61a-2f2037beaf34` completed at 20:27:20Z using the checksummed macOS AMD64 binary through Rosetta. Disposable Alice/Bob sign-in, owner-scoped organization/server reads, foreign server/revocation denial, own revocation enforcement and logout cleanup passed. Tokens remained absent from CLI output, private credential files were removed, and both created credentials were absent from active access afterward.

Only authentication activity changed. Source deployment/job/export/enrollment/access/choice work, both native inventories/custody and the binary hash matched the baseline. No deployment, worker pause, native mutation, image or release publication occurred. Intel hardware, other-platform authorization, installation/signing and live onboarding remain open under issue #27. See [CLI evidence](../local-customer-cli.md).

The translated-authorization full smoke gate passed. PR #86 commit `1911971` passed remote CI run 11586. No actionable findings remain in the local translated-read qualification review. Platform/live release gates remain open.

## Management-owner creation admission

Schema 27 adds audited pause/reopen of new management owners, independently of native deployment, first-account requests and issued invitations. Verified existing issuer/subject sign-in remains available. New creation holds a shared policy lock through owner/session commit; closing waits for admitted work. Same-state retries add no audit, and audit failure rolls back the change. Recovery holds deny both new/existing session creation. Local administration refuses production/remote/customer databases. See [owner admission](../local-owner-admission.md).

Full smoke and focused race gates passed. Tests qualify closed/missing policy, existing sign-in, reopening, audit rollback, admitted-commit closure and held callbacks without session/metadata changes. New policy functions have 81–100% coverage; broader auth/command coverage is 77.5%/29.5%, with actual commands qualified separately. Optional `golangci-lint` remains unavailable.

Actual drill `3b114b18-5b38-4463-a961-51b7cacc254d` completed at 20:45:33Z. Existing Alice/Bob sign-in and server isolation passed during closure. Repeated close/reopen added no duplicates; two effective audits remain and owner admission is open again. Source work and native custody matched the baseline, with intentional authentication activity. New-subject denial remains disposable callback-test evidence; no retained source subject was removed.

Recovery first stopped before backup at PostgreSQL's 100-argument limit in the old inventory builder. Aggregation now handles 51 tables while preserving its private row-hash format. A legacy 50-table digest parity check and 51-table inventory passed. An intermediate held copy remains retained. Final recovery `c7ae795d-df96-425b-b67a-2644024abd6f` copied the 51 current management/River inventory tables, matched sequences, revoked copied authentication and refused held worker startup. Reconciliation/dedicated replay preserved default jobs, source rows/sequences, native data and custody. All management writers remained stopped through capture/replay.

API/worker/monitor resumed on schema 27, readiness is healthy, fixture permissions remain disabled and all three admission policies are open. Six cancelled endpoint choices/twelve audits and zero pending choices remain, with management/native work hashes unchanged. Final post-recovery smoke passed. Native containers, volumes and recovery copies remain retained. Production signup/operator administration, independent storage, live hosting and release evidence remain open. PR #87 commit `9047cbe` passed remote CI 11593.

No actionable findings remain in the owner-admission/local recovery review. The added migration is forward-only and older binaries refuse schema 27. Public opening, production administration and native issued-invitation controls remain unqualified.

## Restricted issued-invitation withdrawal command

The restricted agent now closes an exact retained first-account ticket, confirms closed guard publication, drains ingress/native signup and revokes unused invitations without replacements. Interrupted work retains closed admission and accepts the same-ticket retry. An account completed before/during withdrawal is preserved; a known existing account returns status without restarting containers or changing its lease. See [withdrawal command](../local-enrollment-withdrawal-agent.md).

Full smoke and focused race gates passed. Fixed-helper tests and agent fixtures cover interrupted drain, lost response, ordered drain, idempotent retry, stale-ticket refusal, reopening denial and an in-flight completed account. Actual preservation drill `dd99de2e-2b43-43c5-b88f-ad3b6d459df0` completed at 21:11:22Z against both retained verified accounts. Four repeat reads and scope/configuration denials passed. Source management work, native signed inventory/media, container start times and custody matched the baseline. Schema remains 27. Agent restarted with existing PKI and the same three-slot ceiling; no native container or volume was replaced.

New withdrawal/client functions have 80.4–100% focused coverage; broader node-agent package coverage is 62.0%. Optional `golangci-lint` remains unavailable. The first regression was a compile failure before the command existed, not a behavioral red result. Fresh empty-PDS withdrawal and real drain/race failure qualification remain open. Management intent/audit/queue and browser controls are next; this agent command alone does not complete issued-invitation control or public signup. All retained source accounts, recovery copies and occupied slots remain preserved. PR #88 commit `e525bb4` passed remote CI run 11596.

The local review found no remaining actionable defects in the restricted command, client, fixtures, preservation drill and evidence documents. Customer control and fresh native withdrawal are explicitly deferred qualification steps.
