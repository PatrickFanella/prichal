# Fresh managed allocation on staging-02 — October 30, 2026

This #7 run created a new managed PDS through the production-mode management API, let the staging worker build it on the freed staging-02 slot, carried the export's forwarding handoff to the edge and committed public readiness. It reused only the VM, its enrolled node identity and the dedicated registry. The [retired fixture](managed-fixture-withdrawal.md) keeps its tombstoned hostname, forward 35000, volumes and hold.

The deployment is `e4067ea1-f000-4321-852f-0c67a179c0ed` on node `60684f10-c6b2-4833-a0b8-fb9f96a7869b`. Its serving URL is `https://pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv`. It has zero accounts, and account creation stays closed.

## Binaries

Built from merged main `f2f975f` with the pinned Go 1.27.1 toolchain (`CGO_ENABLED=0`, `-trimpath`):

| Binary | SHA256 |
| --- | --- |
| api | `19704435df864dd010b08c37c13819e228be3c0ebddf1863d1675e1ddb9b887e` |
| worker | `863e44c340113495bad5e6cdd363c3800f5baeb26c8aa622ff0dd408bd59b228` |
| managed-route-admin | `a9bce0f375d66a0d7936fe80cbd928b652932ff52e66535f74198533b8baf1bf` |
| entitlement-admin | `35541240967c2a66d6b93fc847ca12ccaf2c5c5178d226f854aa1c15eb7ca5e0` |

The Almaz publisher (`07c2c008…`) and renderer (`43509b44…`) were unchanged. Renderer source has not changed since the commit that installed it. The guest agent (`63082ef1…`) already had `PDS_STAGING_MANAGED_RUNTIME=true` and was not changed.

## Sequence

1. **Before snapshot.** Schema 42, one failed deployment, no placement, no unfinished jobs, holds or other connections. The dump (`968f9f3cd778bf4c9641238a098a21d5353153df9a237c928866eec99064f6e7`) restored all 70 tables and every sequence into a disposable database, which was dropped.
2. **Grant.** Allocation counts every agent deployment an organization has ever had, including the retired one, so the existing synthetic organization got a 24-hour `local-small` grant (limit 3) from `entitlement-admin` under key `b69dc733-ea6f-4fe8-bd5f-3aac110b7a31`.
3. **API intent.** The real `api` binary ran on Kvant loopback 18092 with `PDS_MODE=production`, `PDS_STAGING_PROVISIONING=true` and `PDS_STAGING_MANAGED_INTENTS=true`. It discovered the real Gitea issuer but used a synthetic client ID/secret. A 20-minute synthetic session for the existing synthetic owner was seeded directly, as in the original staging qualification; no real Gitea login, code exchange or browser session is claimed. The owner deployment route returned 200 with state `queued`, the generated hostname, an empty endpoint and the "public activation is pending" message. Exact-key replay returned the same deployment. The session row was then deleted and the API stopped. The transaction reserved edge forward 35001 with basis `queued` and actor `owner:<synthetic owner UUID>` and queued one job on `staging_managed_deployments`.
4. **Native runtime.** The real `worker` ran with `PDS_STAGING_MANAGED_RUNTIME=true` and without public checks. Within two seconds the deployment moved to `checking` ("Managed PDS resources exist. Public readiness is pending."). The guest had a healthy tenant and an edge container on loopback 44374, the managed identity receipt, two volumes and authenticated capacity listing only the new instance. The retired fixture's hold lease was unchanged.
5. **Forwards.** Kvant user unit `prichal-staging-02-managed-e4067ea1-ingress-tunnel.service` forwards 127.0.0.1:44374 to the guest's 44374. Almaz user unit `prichal-staging-02-managed-e4067ea1-edge-tunnel.service` forwards Caddy bridge address 172.20.0.1:35001 to Kvant 44374. Both were new; both ports were free; the retired fixture's units stayed disabled. Before publication, the native PDS advertised `did:web:pds-e4067ea1f0004321852f0c67a179c0ed.subcult.tv` through the Almaz listener.
6. **Observation and export.** Observation key `1ac3d0cb-cbc6-4663-9541-e001371f14ad` produced generation 10. Its inventory (`d0e1ea91bc983c070d3e8c13728562e3236d2f05214fe08b3167510436295ada`) holds only this endpoint; the released fixture is excluded. `forwarding.json` repeats that digest, generation and deadline and names native endpoint `https://e4067ea1-f000-4321-852f-0c67a179c0ed.localhost:44374`, node key `ec38e724…` and forward 35001, matching the installed units.
7. **Publication.** The bundle was copied to Almaz with modes 0700/0600. With no managed route present, `prepare --confirm-managed-inventory` took the initial-publication path as revision `ad1a6fe8-00ee-4073-9ae6-01b3b92d59ad`, passed the pinned upstream identity checks and validated the full staged configuration. `publish` then reloaded Caddy. Public health went from 404 to 200 (`0.4.5036`), and `describeServer` and OAuth metadata returned the generated identity.
8. **Readiness.** The worker ran again with `PDS_STAGING_MANAGED_PUBLIC_CHECKS=true`. Check cycle 1, attempt 1 passed and committed `ready` with the serving URL. Its receipt records Cloudflare addresses, edge certificate `517382b3…` valid until December 15, native `0.4.5036` and a rejected unknown generated hostname. The job completed on attempt 1.
9. **After snapshot.** Two deployments (fixture failed, new ready), one placement, two endpoint reservations and no jobs or holds. The dump (`54be0160e2fac2596da3aae4c420321904c5096dce3b12af3a21c68c2ff5b1e9`) restored all 70 tables and every sequence; the disposable copy was dropped.

Separate probes after readiness returned WebSocket upgrade 101 on `subscribeRepos`, 404 for an unknown generated host and for the retired fixture's host, and 403 "Account creation is closed" for `createAccount`. Authenticated usage reported zero accounts.

## Custody

Kvant: `~/.local/state/prichal-staging-02-bootstrap/managed-allocation-20261030/` holds the binaries, scripts, API/worker logs, submission and worker reports, the observation, the bundle and both snapshots. Almaz: `~/.local/state/prichal-managed-publisher/allocation-e4067ea1-gen10/` holds the delivered bundle, and `/var/lib/prichal-managed-publisher/ad1a6fe8-00ee-4073-9ae6-01b3b92d59ad/` holds the root-only publication receipts. No session token, CSRF value or credential was written to these reports. The canonical Machinum report and Almaz/Kvant briefing entries are committed as `0016f4a`; the integrity audit reported no errors.

## Limits

- This qualifies one API-created allocation end to end on designated staging capacity. Owner authentication was synthetic; the real Gitea sign-in path was qualified separately on the first staging registry.
- No API, worker or management process runs continuously. The worker stopped after readiness; [Dozor's public probe](#public-monitoring--october-31-2026) now checks the endpoint instead.
- At allocation the node leaf expired October 31, 2026 at 23:11:48 UTC, and renewal was manual. An hourly [renewal timer](staging-02-transport-renewal.md#scheduled-renewal) now handles it, with Dozor alerts; its first unattended renewal was on October 31. Public serving does not depend on the agent leaf, but management readback does.
- The two forwards are user SSH units with `Restart=on-failure`. They depend on Kvant, Almaz and the guest staying up; boot and failure behavior is not qualified.
- Handles, account enrollment, supported DNS/provider failure and withdrawal of this endpoint remain #7 and #8 work.

## Public monitoring — October 31, 2026

Dozor's existing `blackbox-http` job probes `/xrpc/_health` on the serving URL with labels `project=prichal`, `host=kvant` and `deployment=e4067ea1`. [`staging-02-public-alerts.yml`](../deploy/homelab/staging-02-public-alerts.yml), from PRs #227 and #228 (merge `2e314524c8c24e8a2e447a1172700a2aef50d918`), sends warnings for three conditions:

- five minutes of failed probes;
- the probe series disappearing, for example if the target is removed without a withdrawal;
- an edge certificate with under seven days left.

The first install labelled the target `environment: staging`. Dozor's first Alertmanager route sends that label to `discard`, which `amtool` confirmed, so #228 selects the probe by `deployment` instead. The staging discard route itself was not changed. The new labels route to `ntfy-no-resolved`, the receiver for the staging-02 certificate alerts.

Each Prometheus change passed `promtool check config` inside the container before it was written in place, with the inode retained, and reloaded. Prometheus reports probe success and HTTP 200, with the edge certificate valid until December 15, and all three rules are healthy and inactive. No notification from these rules has been delivered. Dozor's private `~/.local/state/prichal-staging-02-public-probe/` holds both previous configurations, validation logs and hash receipts. Machinum `81f3be9` records the probe in the Dozor briefing.

The probe checks HTTP health only. It does not open the `subscribeRepos` WebSocket and does not recheck the closed account creation or the 404 for unknown hosts; those were qualified once at readiness.

## Pause and rollback

To withdraw the route, use the publisher's rollback for revision `ad1a6fe8-00ee-4073-9ae6-01b3b92d59ad`. It requires the installed parent/route bytes, unrelated Caddy inputs and the active configuration to still match its receipt. It restores the pre-publication parent and removes the managed route. Then stop and disable the two `e4067ea1` units, and remove Dozor's probe target, rule-file line and rule file together so the missing-probe alert does not fire. The registry keeps the ready row, so record the withdrawal through a reviewed procedure rather than editing state. Do not delete the deployment's volumes, identity receipt or reservations, and do not restore the before snapshot over later writes.
