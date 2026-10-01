# Management authentication

Management sign-in uses OpenID Connect authorization code flow with PKCE. The Go API completes the exchange and validates the signed ID token. The browser receives an opaque management cookie. PDS login, passwords and DIDs are separate.

The production provider baseline is a dedicated ZITADEL project and Web application, using authorization code with PKCE and `client_secret_basic`. ZITADEL documents its [application types](https://zitadel.com/docs/guides/manage/console/applications-overview) and [discovery and token endpoints](https://zitadel.com/docs/apis/openidoauth/endpoints). The backend also supports a public PKCE client using the advertised `none` method, which the isolated local fixture uses. Client authentication is explicit; the backend does not probe token requests and risk spending the same code twice.

The provider choice is an implementation baseline. No production application, provider account, domain or cloud service has been provisioned. Qualify the real provider before staging access. Another compatible OIDC provider can be configured without changing management owner IDs into emails or DIDs.

## Local startup

Start the database and install the locked frontend dependencies as described in [local development](local-development.md). Then use three terminals:

```sh
./scripts/dev.sh issuer
./scripts/dev.sh api
./scripts/dev.sh web
```

Open <http://localhost:5173>, choose Sign in, then Continue as Alice or Continue as Bob. Each owner can create and list their own organizations. Sign out invalidates this browser's management session; Sign out all devices revokes every management session for that owner.

The fixture command listens only on `127.0.0.1:5556`, requires explicit local mode, signs test ID tokens and permits only the configured client and callback. Both owners have the same test email. It creates no public identity. Its in-memory signing key and authorization codes are disposable. Management sessions persist in PostgreSQL and survive API or fixture restarts.

## Production settings

Run the Go API directly with environment settings from the secret store. The development helper always enables the isolated local configuration and is not a production launcher.

| Setting | Meaning |
| --- | --- |
| `PDS_MODE` | `production`, also the default when unset |
| `PDS_DATABASE_URL` | Management PostgreSQL connection from the secret store |
| `PDS_LISTEN_ADDRESS` | Private API listener, default `127.0.0.1:8080` |
| `PDS_PUBLIC_ORIGIN` | Exact browser origin, HTTPS with no path, query, fragment or trailing slash |
| `PDS_OIDC_ISSUER` | Exact HTTPS issuer from provider discovery |
| `PDS_OIDC_CLIENT_ID` | Dedicated management Web application ID |
| `PDS_OIDC_CLIENT_SECRET` | Confidential application secret, use `client_secret_basic` |

Register exactly `PDS_PUBLIC_ORIGIN` plus `/api/auth/callback` as the application redirect URI. Serve the frontend and `/api` behind the same HTTPS origin. Do not derive callback destinations from incoming Host or forwarded headers. Enable PKCE with S256. Keep application secrets server-side; no provider access or refresh token is retained or sent to the interface.

If deliberately using a public PKCE application, omit the client secret and register the `none` token authentication method. Discovery must advertise the selected method. Authorization, token and JWKS endpoints must use HTTPS in production. HTTP issuer and origin URLs are accepted only in explicit local mode and only on loopback.

## Session and organization rules

- Owners use the unique issuer and subject pair. The API ignores email and DID for identity matching. Switching issuers creates a distinct owner; it does not move existing organizations.
- Login state is hashed in PostgreSQL and bound to an HttpOnly browser cookie. Nonce and PKCE verifier stay server-side. Login attempts expire after ten minutes. Callback consumption is atomic and single-use, including rejected token responses.
- Signed tokens must have an accepted signature, configured issuer and audience, unexpired time and matching nonce. The integration tests use the real OIDC verifier and JWKS rather than bypassing token checks.
- Management session tokens are random and stored only as SHA-256 hashes. Sessions expire after 24 hours. The production cookie is `__Host-pds-session`, Secure, HttpOnly, SameSite=Lax, with `/` path and no Domain attribute. Local HTTP uses a separate `pds-local-session` cookie.
- Creating or renaming an organization and signing out require both the exact Origin header and the session's CSRF token. Session and organization responses disable caching.
- Every organization read, list and update includes the authenticated owner ID in its database query. Foreign IDs and missing IDs both return 404. Caller-supplied owner IDs are rejected.
- Sign out revokes the current stored session. Sign out all devices revokes all stored sessions for that owner. These actions affect management sessions. Upstream provider SSO is independent; provider back-channel logout and administrator-driven revocation hooks are not implemented in this slice.

## Migration and verification

Schema version 2 adds owners, login attempts, management sessions and organization ownership. The version 1 foundation has no organization creation endpoint. If someone manually inserted unowned organization rows, migration 2 fails rather than assigning them to an arbitrary owner. The transaction rolls back and retains those rows and version 1. Reconcile such data explicitly before retrying; do not delete or guess ownership.

Run `./scripts/dev.sh smoke`. It builds the frontend, runs Go vet and race-enabled tests against isolated disposable databases, and checks successful public/confidential client flows, bad signatures/issuer/audience/nonce/expiry, wrong PKCE, expired state, browser binding, callback replay, session reconstruction, logout replay, all-session revocation, CSRF and cross-owner isolation. It also exercises version 1 upgrades and rollback on unowned data.

Local browser verification covers Alice and Bob, organization creation, sign-out, absence of foreign organizations and persistence across API restart. Live ZITADEL registration, HTTPS browser cookie behavior and provider-side SSO policy still need staging qualification.
