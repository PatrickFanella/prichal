# Hosted preview

Two hostnames run from one stack on Dozor (`10.0.0.57`), behind the Almaz Caddy edge:

| Host | Serves | Access |
|---|---|---|
| `prichal.subcult.tv` | Static landing page from `site/` | Public |
| `prichal-app.subcult.tv` | Management interface and API | Authelia, then Gitea sign-in |

This is a preview, not public PDS hosting. The API runs in production mode with Gitea (`https://git.subcult.tv`) as the OIDC issuer. Organizations and the status page work. Deployment, enrollment, exports, billing and other local fixtures stay disabled in production mode, and no worker or node agent runs. Owner admission starts open, so anyone who passes Authelia and signs in with Gitea becomes an owner.

## Layout on Dozor

- Releases: `/srv/apps/prichal/releases/<commit>/source` (a clean checkout), with `current` linking to the active release.
- Secrets: `/etc/prichal/production.env`, owner-only, with `PRICHAL_DB_PASSWORD`, `PRICHAL_OIDC_CLIENT_ID` and `PRICHAL_OIDC_CLIENT_SECRET`. Set the OIDC pair with `deploy/production/set-oidc-client.sh` from the current release; it reads the secret without echoing it, keeps a dated copy of the previous file and restarts only the API. The Gitea OAuth application "Prichal hosted preview" belongs to the PatrickFanella account; its only redirect URI is `https://prichal-app.subcult.tv/api/auth/callback`.
- Compose project `prichal` from `deploy/production/compose.yaml`: PostgreSQL 17 (volume `prichal_management-data`), the API and a Caddy web listener. Only `10.0.0.57:3090` is published. PostgreSQL has no published port.

## Release

```sh
ssh dozor
REV=<commit>
git clone --quiet https://git.subcult.tv/PatrickFanella/pds-ass.git /srv/apps/prichal/releases/$REV/source
cd /srv/apps/prichal/releases/$REV/source && git checkout --quiet $REV
cd deploy/production
set -a; . /etc/prichal/production.env; set +a
PRICHAL_REVISION=$REV docker compose build
PRICHAL_REVISION=$REV docker compose up -d
ln -sfn /srv/apps/prichal/releases/$REV /srv/apps/prichal/current
```

The API applies management migrations at startup and refuses a database with a newer schema. Before a release that adds a migration, dump the database: `docker exec prichal-postgres-1 pg_dump -U prichal prichal > /srv/apps/prichal/backups/<date>.sql`.

Check: `curl -H 'Host: prichal-app.subcult.tv' http://10.0.0.57:3090/api/readiness` returns `"ready":true` with the expected schema version.

## Private preview backups

The fixed-source helper is installed at `/srv/apps/prichal/operations/preview-backup.py`. `prichal-preview-backup.timer` runs nightly at 02:40 Central plus up to ten minutes, retaining 14 generated backups after successful isolated restore verification. A manual live-source backup and matching-image application-readiness restore passed on October 19; the first natural timer run passed on October 20 at 02:43 CDT, independently matching the archive checksum and restoring 60 tables at schema 35 before stopping its isolated verifier. See the [backup runbook](hosted-preview-backups.md) and [rollout evidence](live-preview-backup-rollout.md). Owner-admission closure, monitoring/alert qualification and direct release fetching remain unverified by this rollout; the owner subsequently closed #127.

## Rollback

Start the previous release's images with its commit: `PRICHAL_REVISION=<previous> docker compose up -d` from that release's `deploy/production`, then point `current` back. A migration cannot be undone this way; restore the dump into a fresh volume instead. To take the preview offline, run `docker compose down` (the volume is kept) and remove `sites/prichal.Caddyfile` and its import on Almaz.

## Edge

Almaz `/opt/server/management/config/caddy/sites/prichal.Caddyfile` proxies both hostnames to `10.0.0.57:3090` and preserves the Host header. The app host imports the `authelia` snippet. `*.subcult.tv` already reaches Almaz through the Cloudflare tunnel, so no DNS change is needed.
