# Public staging management — October 11, 2026

The owner selected `https://prichal-staging.subcult.tv` for the Kvant staging dashboard with SUBCULT Gitea sign-in. The new protected route and production-authentication API are live. Real user sign-in/callback and the authenticated public dashboard remain pending the owner's Authelia/Gitea session; server startup and redirects do not prove that browser flow.

PR #145 is verified merged at `f009c1a084892c6226fe835acd1e05a42b5d3c88`. The independent connected-node recovery fixture remains stopped. No existing PDS identity, route or production management release was replaced.

## Separate runtime and identity policy

A dedicated confidential Gitea application, ID 6, named `Prichal Kvant staging`, registers only `https://prichal-staging.subcult.tv/api/auth/callback` and retains the provider's explicit consent step. It was created through the [Gitea OAuth application API](https://docs.gitea.com/api/1.25/operations/user-create-oauth2-application/), with the generated secret retained privately outside Git.

Public runtime state is under `~/.local/state/prichal-public-staging/`, mode 0700; environment, OAuth custody and operator policy files are 0600. Its API uses `PDS_MODE=production`, issuer `https://git.subcult.tv`, the exact HTTPS public origin and loopback listener 8088. The frontend listener is loopback 5178 under an enabled user service. Source commit `cd862584fb3832fc608590abd6fe6c81e55e53fa` supplies the API and fixed frontend proxy. API image `sha256:455028917e51c5469b2cf0bb80aa284ff0f1f972e9c9761427db9c1aa61586ff` includes public system trust roots for HTTPS provider discovery.

The public API uses the existing separate staging management PostgreSQL registry. The private synthetic-identity API/worker/observer and PDS remain independent running processes with their original origin and settings. Real Gitea owners are distinct issuer/subject identities; synthetic Alice/Bob organizations are not adopted through email or display-name matching.

`PDS_OPERATOR_FILE` enables a bounded private production inspection policy. It requires production mode, an HTTPS issuer and no simultaneous local policy. Every configured identity must match the configured issuer and have an explicit subject; only `inspect` is accepted. `PDS_LOCAL_OPERATOR_FILE` still refuses production mode, and the worker still requires local mode. Production provisioning, enrollment, local grants and mutation policies were not enabled by this change.

The staging inspection policy names Gitea subject `1`, selected from the existing PatrickFanella account's immutable Gitea user ID. Actual verified callback subject agreement remains pending sign-in. Authorization resolves the authenticated management owner's issuer/subject; request email, display name, client bearer or body role cannot confer rights. No synthetic organization ownership was changed.

The API has one read-only mount: its dedicated operators directory. It receives no Docker socket, SSH key, CA signer, node client key, enrollment cipher or native PDS credentials. The container has a read-only root, dropped capabilities, no-new-privileges and 256 MiB/half-CPU bounds. There is no public worker container.

## Edge and transport

Almaz's enabled `prichal-public-staging-edge-tunnel.service` uses existing pinned host SSH to Kvant. It binds only the existing Caddy bridge gateway `172.20.0.1:34944`, forwarding to Kvant loopback 5178. The fixed frontend proxy accepts only Host `prichal-staging.subcult.tv`; foreign hosts return 421. Its API upstream is fixed loopback 8088; no incoming Host determines OAuth destinations.

A new imported `prichal-staging.Caddyfile` uses existing Authelia protection. Only an import was appended to the existing Prichal route file. Full Caddy validation and graceful reload passed; the main Caddyfile remained byte-identical. Prior/candidate route files, logs and activation hashes are retained at Almaz's private `~/.local/state/prichal-public-staging-edge/`. Active parent-route SHA-256 is `0b5bbe96c732f8bc0c8ee62a0cc290d3db7565c128d469cc10b8b890f586f910` and the new route hash `3517644d7f789f565b2dd2dcc49730860e9e6ba22d21b08af5b82e0deb35346d`.

Existing wildcard DNS and Cloudflare TLS served the selected hostname without a DNS or credential mutation. An unauthenticated public request returned 302 to `auth.subcult.tv` with this exact staging return URL. The collaborative preview rendered Authelia login. This qualifies protected public routing, not successful Authelia or Gitea sign-in.

Direct upstream qualification returned 401 for a missing management session, 403 for CLI bearer operator inventory and 421 for a foreign Host. API login returned the HTTPS Gitea authorization endpoint, exact registered callback and S256 PKCE. State-cookie flags were Secure/HttpOnly/SameSite=Lax, host-prefixed, Path=/ and without Domain. Cookie/state values remain private. The completed real provider callback, persisted public session, inspector-only action refusal and authenticated dashboard are still required.

## Custody, verification and rollback

A separate encrypted configuration checkpoint includes the new public runtime, frontend and OAuth client custody; it does not include the VM or registry. Ciphertext is retained on Kvant and at Dozor's `/srv/recovery/kvant/prichal-staging/public-management-20261011.pvm`, SHA-256 `6d7711746bf3c0d64fa96cf43dc3d503f5d4fa01550bf38928af1e2bb5619d74`. Existing independent private recovery-key copies remain required. Retrieval and authenticated decryption passed; generated plaintext archives were removed. Restoring the older VM/registry checkpoint needs this newer configuration custody and subsequent registry changes reconciled separately.

Full `./scripts/dev.sh smoke` passed against disposable PostgreSQL using pinned toolchains: 44 Node checks, web build, Go vet and all race-enabled packages. Production inspection tests cover exclusive configuration, HTTPS issuer, explicit subject, local-mode refusal and mutation-role refusal; existing local policy guards passed. Fifteen homelab tests, Node syntax and Compose validation passed. The smoke database was stopped with its volume retained.

For route rollback, compare current parent/new route hashes and subsequent edits to the private receipt before restoring only the preceding Prichal route and validating/reloading Caddy. Keep the main Caddyfile and existing PDS routes intact. Stop only the new Almaz tunnel and Kvant public frontend/API if pausing this surface; retain its OAuth application, secret and configuration. Do not revoke unrelated clients or roll back the staging registry over later writes.

Fresh original NUC and Dozor pilot native health returned 0.4.5027 and 0.4.5036. The private staging registry/worker/node and stopped recovery fixture remain separate. Public native PDS routing, populated-account recovery, production provisioning, scheduled checkpoint acquisition, naturally due renewal/alerts and unattended boot/cutover remain open.
