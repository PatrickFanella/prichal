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
