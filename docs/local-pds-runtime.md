# Disposable local PDS runtime

This fixture runs the official Bluesky PDS through Caddy on <http://localhost:2583>. It is separate from management PostgreSQL. It creates no public identity. The optional account journey creates a retained `.test` account in an isolated identity directory.

```sh
./scripts/pds-local.sh up
./scripts/pds-local.sh check
./scripts/pds-local.sh qualify
./scripts/pds-local.sh qualify-account
```

`qualify` creates a one-use invitation, checks SQLite integrity, writes a temporary blob-volume canary, restarts the PDS, verifies persistence, stops the PDS to check the proxy's failure response, and starts it again. The canary is removed afterward. The unused invitation remains in this disposable fixture. The command restarts only the `pds-ass-runtime` PDS; run it when no other local work is using this fixture.

The manifest is [deploy/local/compose.yaml](../deploy/local/compose.yaml). The pinned upstream image resolved to PDS version `0.4.5036` in local verification on 2026-09-30. Caddy and PostgreSQL also use digest pins; changing a tag alone does not upgrade them. Requalify an intentional digest update.

## Isolation and limits

- The PDS has no published port. Caddy exposes only `127.0.0.1:2583`, permits `localhost` and `127.0.0.1` host headers, and proxies HTTP and WebSockets. Its admin API is disabled.
- PDS networking is internal to Docker, with no external network attachment. Its PLC URL points to the dedicated local protocol fixture, crawler targets are empty, and handles use `.test`. Caddy has a separate ingress network for its loopback port; the PDS remains on the internal network.
- Generated JWT, admin and rotation secrets are retained in ignored `.local/pds.env` with owner-only file permissions. Do not print or commit it. Keep it with the retained volumes.
- The PDS has separate `pds-data` SQLite and `pds-blobs` volumes. Neither contains management data. Restart and `down` retain both.
- CPU, memory and PID caps are set in Compose: PDS 1 CPU, 768 MiB and 128 PIDs; ingress 0.25 CPU, 128 MiB and 64 PIDs. Container capabilities are dropped; ingress retains only the bind-service capability required by the upstream Caddy binary. Neither service mounts the Docker socket.
- Invites and upstream rate limits remain enabled. Blob upload size is capped at 5 MiB. The basic qualification creates no user accounts. `qualify-account` creates one local fixture per invocation. Disk quotas and an enforceable per-tenant account cap remain unimplemented and must be resolved before shared hosting. Docker Desktop named volumes do not enforce a tenant disk quota.

The ingress uses local HTTP. Public DNS, ACME/TLS and live federation are separate gates; do not reuse this development manifest as a production deployment.

## Evidence limits

The health endpoint is an upstream process check. The qualification script separately reads actual SQLite integrity and verifies an invitation through the admin protocol. The blob canary proves volume persistence, not an authenticated upload or repository record. Storage exhaustion and corrupted database behavior still need isolated fixtures. The separate account journey now verifies authenticated uploads and repository data. Do not mark issue #4 or M1 complete from these checks alone.

The local protocol fixture uses the pinned PDS image's upstream PLC library to validate creation, signatures and operation history. It accepts only localhost PDS endpoints and rejects forged updates. It captures SMTP mail only for `.invalid` recipients, without delivery. Identity history and captured mail persist in a separate `protocol-data` volume. Its HTTP inspection port is loopback-only at `2582`; captured messages contain verification codes, so treat this volume as private. The fixture has internal and loopback ingress networks; the PDS has only the internal network and cannot contact public `plc.directory`. Do not create production identities in CI. For live qualification, select a designated staging PDS domain, approved email sender and disposable live account explicitly, then record DID/handle resolution, record/blob reads, stream events and downstream visibility separately.

## Stop and recover

```sh
./scripts/pds-local.sh down
```

This removes only fixture containers and networks, retaining its volumes. `up` reuses the same secrets and data. Do not remove the volumes or `.local/pds.env` independently. Public DNS, live host selection and cloud spend remain undecided.

## Account qualification

`qualify-account` verifies fresh account creation, password login through the PDS, one-use invitation rejection, handle/DID/PDS resolution, captured email confirmation and confirmation-token reuse rejection. It uploads a one-pixel image, writes a single post referencing it, reads both back and checks the matching WebSocket commit through Caddy. Export validation uses the pinned upstream repository library, verifies every CAR block CID and commit signature, and rejects a wrong key and a tampered export.

The script restarts only the qualification PDS and protocol fixture. It then verifies login, confirmed email, identity, record and uploaded media persistence. It retains the account and record for later recovery drills. Passwords stay in owner-only `.local/account-fixtures/` files; access/refresh tokens are not written or printed. Run this command only when the isolated fixture is not in use.

This is protocol and transport evidence on localhost. It does not implement owner-authorized product enrollment, enforce shared-hosting account/disk limits, approve a production email sender or prove discovery/indexing by an external relay. Local managed agent PDS instances still have public identity disabled and do not use this fixture.

## Local registration expiry

The pinned PDS supports one-use invitations without an expiry field. The qualification ingress therefore checks a separate 15-minute lease before both XRPC and upstream OAuth registration. It forwards a GET authorization check and does not send the password body to the guard. The lease is checked on every request. Missing, malformed, wrong-tenant, future-issued, excessive-duration and expired policies deny registration. An unavailable guard also blocks registration while existing PDS health remains available. Case, encoded-path and trailing-slash aliases are checked.

The operator-only fixture broker closes registration, revokes retained unused native codes, checks its 32-repository qualification ceiling, issues one native invitation and atomically publishes its bounded lease. Recovery cannot reactivate an old unused code. The account journey closes registration after verifying code reuse rejection. Existing accounts can still log in and write data.

Qualification runs hold an exclusive local directory lock. A hard-killed run leaves `.local/account-fixtures/qualification.lock` for review; check that the previous process is absent before removing that empty generated lock directory. Do not run qualification concurrently.

This is a fixture guard, not the management enrollment API. Product work still needs owner/session authorization, persisted request/recovery limits, worker-to-agent integration, invitation status and per-instance guard policy. Managed agent PDS instances remain closed to identity work until their advertised endpoint matches their ingress and local identity/mail fixtures are configured. The expiry check is admission-time validation; it does not cancel an already accepted upstream request.

The guard uses Caddy's documented [forward authorization](https://caddyserver.com/docs/caddyfile/directives/forward_auth) and [decoded regular-expression path matching](https://caddyserver.com/docs/caddyfile/matchers#path_regexp). The pinned Caddy binary validated this configuration during local qualification.
