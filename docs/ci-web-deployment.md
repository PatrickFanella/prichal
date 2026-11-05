# Automatic hosted-preview web delivery

The required `Verify local foundation / smoke` job keeps the existing disposable
database, Go, frontend, recovery-boundary and CLI checks. It also builds and
checks the actual production Caddy image. Only a successful canonical main push
or main workflow dispatch can receive and activate that image on Dozor. Pull
requests exercise the image without deployment credentials or production access.

The public site comes from the checked-out main commit's `site/` directory.
The same listener also serves the protected management interface. Its admitted
source is recorded in `deploy/production/web-release.json`: commit
`decd4442c8dba11b101c7165213fcfaec67b06ed`, matching the retained schema-35 API.
CI checks out that exact source without persisted Git credentials and supplies
it as `MANAGEMENT_SOURCE` to the web Dockerfile. The resulting eight management
files match the retained interface byte-for-byte. Current main's newer
management features require schema 42 and are not promoted by this web job.

The Dockerfile's default remains the current checkout's `web/`, for the separate
native full-stack release procedure. Its CI management pin must be updated only
after a compatible native API/database release is qualified. Public hosting,
sign-up, worker execution and managed PDS admission retain their existing gates.

## Receiver and admission

Gitea holds only `HOMELAB_DEPLOY_KEY` and `HOMELAB_KNOWN_HOSTS` for this job.
The operator owns the key; its review/rotation date is February 3, 2027. Dozor's
authorized key accepts only Kvant's verified LAN source and a fixed receiver
with the `prichal-web` profile. It refuses a shell and API-service selection.
Kvant and the actual Kuznya CI guest passed the key probes.

The pinned receiver is `ecc088dad74351b536f156b351cf199e95e3f220` with SHA-256
`dacc492837156415955878b3eabc45466cf594f0d3ddc93d95b839fc799f4a3b`.
The private profile and resolved web-only Compose baseline are
`~/.config/fleet-deploy/prichal-web.json` and
`~/.config/fleet-deploy/prichal-web-compose.json`. They preserve the web
listener, resources, isolation, tmpfs and existing networks. They contain no
management credential values and do not resolve unrelated services on deploy.
Registration and recovery receipts are under
`~/.local/state/fleet-deploy/prichal-web/`.

The receiver requires the current main revision, immutable OCI source identity,
an app deployment lock and the registered runtime guard. Main is checked again
before activation. A retry reuses its first accepted image. The fixed read-only
`check-web-api.py` hook additionally requires the candidate's management source
to match the retained API release, the original native environment-file hash,
the registered API image and unchanged ready/schema response. A mismatched
management image was refused in qualification before activation.

Install that hook from the checked, merged source at the exact hash/path in the
host registration before allowing the first main promotion. A missing hook fails
before activation; install it and retry the failed main run using the accepted
artifact. Do not bypass it or rebuild a different image under the accepted SHA.

Activation selects only Compose service `web`, with dependencies and implicit
builds disabled. API, PostgreSQL, managed agents, certificates and PDS instances
are outside this receiver's selectors. A failed activation restores the prior
immutable web image and checks its public HTML/icons. No database migration or
data restore occurs in this path. A full management release requires its native
backup, restoration and migration admission.

## Verification

`scripts/check-ci-web-image.sh` starts a disposable, read-only Caddy container
with networking disabled and an internal lifetime limit. It checks OCI identity,
all selected landing assets and metadata, plus the admitted management shell and
its assets through the correct Host headers. The fixture is removed on exit.

After promotion, the receiver compares complete public landing HTML, favicon,
touch icon and preview image against immutable image bytes. CI also checks the
accepted root hash and source-selected icons through the live URL. The separate
operator readback must verify the protected app's authentication boundary,
internal API readiness, all public build assets and unchanged related services
before reporting the deployment complete. Local smoke/image success establishes
those local checks; it does not prove a production deployment or broader PDS
qualification.
