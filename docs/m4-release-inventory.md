# M4 release inventory and recovery preparation

## Inspection scope — October 28, 2026

This is a source inspection of `a06fbc46098076362a8dcc6f0e5f7689c09c6ff6`, whose merged-main smoke check passed. It identifies inputs for #25 and #26; it is not a release designation, running-service inventory or deployment authorization. No image was built, published or pulled, and no service, secret, backup or DNS record was changed.

The owner retains M4's independent serving/recovery requirement and uses the homelab as an interim private pilot. Provider/account, region, budget, general managed namespace, production identity/mail ownership and independent backup custody remain unresolved. Keep those fields explicitly unselected in any proposed release record. See the [M4 gate](m4-release-gate.md).

## Artifact inventory

| Responsibility | Existing source artifact | Current boundary and missing release evidence |
| --- | --- | --- |
| Management API | [API Dockerfile](../deploy/production/api.Dockerfile), [API entry point](../backend/cmd/api/main.go) | Digest-pinned build/runtime bases; builds only the API. The preview Compose service selects `prichal-api` by revision tag, not immutable application digest. Record a published image digest and target-platform smoke result before selecting a production release. Startup applies migrations and checks the recovery fence; this is not a migration rollback procedure. |
| Web and ingress | [Web Dockerfile](../deploy/production/web.Dockerfile), [Caddy configuration](../deploy/production/Caddyfile) | Builds the frontend and bundles the landing site. Existing host routing names the fixed preview domains; TLS terminates outside this listener. Record the resulting application image digest and verify the selected public origin, proxy/TLS chain, OIDC callback and security headers in the selected environment. |
| Durable worker | [Worker entry point](../backend/cmd/worker/main.go), [staging management image](../deploy/homelab/management.Dockerfile) | Worker startup explicitly rejects modes other than `local`. The scratch staging image copies prebuilt binaries; its Dockerfile alone does not produce them. A non-local worker path, reproducible build/publication and recovery qualification are missing. Do not remove the mode guard or reuse staging authority merely to populate a release matrix. |
| Restricted host execution | [Pilot agent runbook](production-pilot-agent.md), [staging agent unit](../deploy/homelab/staging-agent.service) | Existing agent authority and units serve bounded pilot/staging paths. Select a reviewed agent artifact and host configuration, approved PDS image and resource limits; verify worker-to-agent identity and restricted operations in the selected fleet. API containers must receive neither Docker access nor host credentials. |
| Management PostgreSQL | [Preview Compose](../deploy/production/compose.yaml) | PostgreSQL 17 image is pinned by digest and management data uses its own volume. This does not include native PDS SQLite/blob storage. Record the production management backup, schema, restore target and independent custodian separately from every PDS's data and identity custody. |
| Preview backup | [Backup helper](../deploy/production/preview-backup.py), [service](../deploy/production/prichal-preview-backup.service), [timer](../deploy/production/prichal-preview-backup.timer) | Fixed preview source container and `/srv/apps/prichal/backups`; bounded dump/isolated-restore checks and 14 retained archives. It is not a general independent backup destination or PDS backup system. Existing natural-run evidence is in the [preview backup report](live-preview-backup-rollout.md); this inspection did not rerun it. |
| Recovery rehearsal | [Restored management Compose](../deploy/homelab/restore-management.compose.yaml), [connected recovery](live-staging-connection-recovery.md), [release rehearsal](local-release-rehearsal.md) | Existing manifests use fixed images, local fixture identity and isolated networks. Preserve these as dated rehearsal inputs. They do not recreate an independently selected production environment or qualify a distinct-release schema downgrade. |

## Configuration gaps that block promotion

The directory name `deploy/production` does not establish production independence. Its Compose header identifies a hosted preview on Dozor. The manifest publishes the web port on Dozor's fixed LAN address, sets the fixed preview origin and uses the homelab Gitea issuer. Its Caddy configuration expects the existing external TLS/edge path. It includes PostgreSQL, API and web, with no worker or host-agent service. Moving this file unchanged to a VPS would not provide the complete managed service.

The [verification workflow](../.gitea/workflows/verify.yaml) runs source, boundary, smoke and customer-CLI build checks. It has no API/web/worker/agent image publication or production rollout stage. Pinned base images and green source CI do not supply published application digests.

The API's [export artifact loader](../backend/internal/exports/artifact.go) permits empty configuration, which disables exports, and rejects configured local artifact storage outside local mode. Production export custody therefore needs an accepted implementation and configuration; copying the local export directory/key settings into the preview manifest is not a supported solution. Keep this tied to #21 and the launch journey.

## Release record to prepare after environment selection

Each designated candidate needs one versioned record with the following fields filled from real evidence:

- Source commit; exact CI run and result; API, web, worker and agent registry references, immutable digests and platform; approved upstream PDS digest.
- Configuration revision and environment identity; selected origins/domains and DNS owner; TLS termination and renewal; issuer/client/callback; mail sender; billing mode; distinct staging/production inventories. Store references to secret versions, never secret values.
- Management schema before/after; migration compatibility and write-fence procedure; per-PDS storage/image compatibility; encrypted backup receipts; archive hashes; key custody and independent restore location.
- Resource/admission limits and enforced plan quotas; readiness checks and observation window; support/on-call owner and hours; independent operator access and escalation path.
- Exact restore and rollback artifacts, selected target inventory, bounded interruption, stop conditions and authorization window. Retain unresolved fields as blockers rather than substituting pilot settings.

## Deployment and rollback review sequence

1. Select the environment, spend ceiling and accountable custodians. Identify designated disposable tenants and keep public signup closed. Produce environment-specific configuration and commands for review before execution.
2. Build and publish the reviewed immutable artifacts. Verify their readback digests and platform, then qualify configuration, secret delivery and migrations in the separate staging environment.
3. Capture and independently restore management state and each designated PDS's data/identity. Preserve manifests, checksums, schemas, queue/recovery-fence results and private credential custody. A dump existing on the source disk is insufficient.
4. Recreate the production-design environment with the workstation and homelab unavailable. Verify auth, billing, mail, managed deployment, account creation, federation and operator recovery through independent paths. Record actual outcomes and failures.
5. Before promotion, review the measured rollback path. If a candidate may migrate data, stop its writers and restore compatible storage before starting an older binary. Do not assume a previous image can open candidate-modified databases. Preserve failed-candidate data for review.
6. Execute only within the separately authorized deployment window. Observe health and supported customer journeys, rehearse dependency failure and operator signup closure, and retain accountable launch sign-off. Any failed critical gate keeps signup closed.

These steps describe required evidence and ordering. No provider-specific infrastructure state, production commands or approved spend is implied. Existing native and public-pilot reports remain useful but cannot replace the independence drill, full #24 journey or #26 launch acceptance.
