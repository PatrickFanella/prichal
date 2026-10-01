# Local development

Use Go 1.27.1, Node 24.15.x, pnpm 12.6.x, Docker with Compose, Bash and OpenSSL. Go modules, JavaScript packages, CI actions and PostgreSQL are pinned. No cloud account, production identity or existing Subcult application is required.

From the repository root:

```sh
cd web
pnpm install --frozen-lockfile
cd ..
./scripts/dev.sh db
```

In two separate terminals:

```sh
./scripts/dev.sh api
```

```sh
./scripts/dev.sh web
```

Open <http://localhost:5173>. The interface polls the Go API through Vite's same-origin proxy every three seconds. The API returns readiness only when PostgreSQL responds with the supported schema version. Responses contain no connection strings or database errors.

Local PostgreSQL binds to `127.0.0.1:35433`; the API defaults to `127.0.0.1:8080` and Vite to `127.0.0.1:5173`. The Compose project is `pds-ass-local`. These ports and resources are separate from other projects. A generated development database password lives in the ignored `.local/local.env` file with owner-only permissions. Keep that file with the database volume: replacing the file does not change an existing PostgreSQL password.

## Verification

```sh
./scripts/dev.sh smoke
```

This installs from the lockfile, builds the interface, runs Go vet and race-enabled tests, and creates a uniquely named disposable smoke database. The smoke test applies migrations twice, checks real database-backed readiness, rejects a future schema version, and checks an unavailable connection. It drops only its own smoke database. The configured parent database must be named `pds_local` or `pds_ci`.

The Gitea workflow runs the same gate against a new PostgreSQL service using disposable CI credentials. Cloud secrets are unnecessary. A queued workflow is not a verified CI result.

To check failure recovery in the browser:

```sh
# In a terminal that loaded the local environment through scripts/dev.sh db:
# Or use the container name below without loading credentials.
docker stop pds-ass-local-db-1
# The page should change to Service unavailable within a few seconds.
docker start pds-ass-local-db-1
# It should return to Service available without a page reload.
```

Stop the local database with `./scripts/dev.sh down`. This retains the management data volume. Stop the API and interface terminals with Ctrl-C. Do not use `docker compose down -v` unless you intend to discard this project's local management data.

## Current boundary

The status page is a working foundation. Owner authentication, server provisioning and public PDS service are subsequent pieces. The local HTTP listeners are for workstation use. Live host selection, public domains, TLS, email delivery and federation qualification remain pending.
