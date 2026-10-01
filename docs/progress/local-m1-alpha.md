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
