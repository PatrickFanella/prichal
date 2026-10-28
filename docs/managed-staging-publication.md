# Restricted managed staging publication

This unit continues #7 after PR #207 merged as `939dfb2624ca3ad9e607aeb4b054e2bd9f588693` with its required check passing. It publishes only deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f` on the owner-authorized disposable staging-02 node. Its retained hostname is `pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv`; the reserved edge forward is 35000. Accounts and handles remain closed. Management remains checking with an empty serving URL until durable public readiness is implemented.

## Implementation and execution plan

1. Inspect the current Almaz Caddy configuration, mounted inputs, active configuration and wildcard Cloudflare route. Record baseline hashes and private receipts. Refuse active/disk drift or conflicts; preserve the main Caddyfile, pilots and unrelated services.
2. Add immutable UUID-scoped trust paths to the existing candidate renderer. The root-only staging publisher accepts a fresh authoritative single-endpoint export plus its independently delivered SHA256, verifies generated identity and the pinned TLS upstream, and validates the complete staged Caddy configuration. It refuses arbitrary endpoint/handle input and existing managed routes. Broader inventory updates are outside this first version.
3. Install only the dedicated SSH forwarding units: Kvant loopback 45109 to the existing guest ingress, and Almaz Caddy bridge gateway 35000 to that loopback listener. Preserve existing SSH host-key checks and keys. No native resource, public DNS or Cloudflare configuration change is needed when the existing wildcard is verified.
4. Prepare an exact-host managed child import beneath the existing Prichal parent file. Keep the original parent and child state, all unrelated input hashes, before/after adapted JSON, fresh inventory, public trust and command logs in root-only receipts. Validate before mutation; load the exact full JSON with a root ETag precondition and verify active readback. Drift refuses publication or rollback.
5. Exercise public readback, a conflicting/stale operation refusal, hash-bound rollback and a new fresh-export publication. Check exact HTTPS/service/OAuth identity, closed signup, WebSocket behavior and unknown hosts. Public probe results are observations, not durable management readiness. Record all failures and preserve retained candidates/rollback files.
6. Run focused tests and the pinned smoke gate, update repository and canonical Machinum evidence, and open the next PR. #7/#126 remain open until their remaining public readiness, DNS custody and failure/retry criteria are verified. Native accounts/default handles remain #8.

## Preflight observations — October 28, 2026

The live Almaz Caddy image is `sha256:a94d181ab9b5de199d3a667760d5743581d87525143c4a9f210a154f0d340911`, reporting 2.11.2. Adapted disk configuration equals active configuration. The main Caddyfile SHA256 is `096f9d2c0f00f10e1449fd65a716445cdbbdf0b523df15d456d28557e2b744f4`. Its existing Prichal parent is root-owned mode 0644 and has a single existing main import. The reserved 35000–35999 range and Kvant loopback 45109 were unused.

The existing local Cloudflare tunnel configuration contains `*.subcult.tv` and `http://caddy:80`; the generated first-level hostname resolves to Cloudflare addresses. This verifies service configuration and resolution, not registrar/account ownership. No zone write or domain purchase is planned.

An isolated network-disabled container using the exact live Caddy image accepted a full root-config POST with exact readback, then rejected its stale root ETag with HTTP 412 and unchanged configuration. It had no host ports and was removed by exact ID/label. Initial attempts encountered an empty startup configuration and a private bind-file permission failure under dropped capabilities; the minimal fixture was corrected before this passing result. The live Caddy process was unchanged by these checks.

## Pause and recovery

Stop the dedicated managed worker before operational changes. To withdraw this route, use its exact root-owned rollback receipt while the installed parent/child, unrelated source inputs and active JSON still match. Restore only the recorded parent/child bytes, validate the full prior configuration and reload with its current ETag. Retain all immutable trust directories, source bundles and logs. A changed unrelated or active configuration requires reviewed recovery rather than overwriting it.

After route withdrawal is verified, stop only the two dedicated managed staging forwarding units; preserve their files and existing VM/native/registry state. Do not stop Caddy, cloudflared, the existing pilot tunnels or Docker. Publication rollback changes no native data or management schema.

The publisher's 15-minute export deadline controls a new publication attempt. Existing routes do not automatically expire when that bundle expires; certificate renewal, ongoing health/withdrawal policy and durable readiness remain separate work. No API or ordinary worker receives DNS, Docker or SSH authority.

## Qualified result — October 28, 2026

The public endpoint passed verified HTTPS, native PDS `0.4.5036` health, exact `did:web` service identity, exact OAuth issuer/authorization/token endpoints, zero repositories, closed native/OAuth signup and a WebSocket 101 handshake with the expected acceptance digest. An unregistered generated hostname returned 404. These were public network observations from the development host, not persisted management readiness.

The first generation-5 attempt refused before active reload because Caddy automatically hid the staged Caddyfile path in two existing file-server handlers. The publisher restored the exact parent/child inputs and verified the original full active configuration. Diagnostic adaptation without reload identified only these hide-path differences. The corrected candidate translates that exact automatic staged-main path to `/etc/caddy/Caddyfile` only within file-server hide lists, preserving every other JSON field. The full final adapted configuration must still match exactly.

Generation 6 published as revision `bf1cd018-2b94-466e-861f-842fe743d87a`; identical-operation replay returned the retained result. Its hash-bound rollback restored the original full configuration, and the public endpoint returned 404. A fresh observation/export then published generation 7 as revision `9a0b24a6-a60e-4097-af0d-50165df7a131`. The same hostname, forward, node, native credentials/certificate, data volumes and placement were retained. Final public probes passed again.

Actual root-operator attempts using an existing managed route, changed certificate bytes and an expired export were refused with unchanged active configuration and source inputs. The isolated Caddy check separately verified stale ETag HTTP 412 refusal. These qualify publisher custody, conflict and reload guards; they do not qualify durable management retry after a DNS/provider certificate outage.

Final readback matched the installed route, adapted disk configuration and full active JSON. Every unrelated configuration/trust input and the main Caddyfile hash stayed unchanged. Only the dedicated managed child route and one appended import in the existing Prichal parent changed. No Caddy/cloudflared/container restart or DNS/Cloudflare configuration write was performed. The two dedicated forwarding units remain enabled; management readback is `checking`, null serving URL, one placement and one conversion record. Existing pilots and unrelated routes were preserved.

| Retained artifact | SHA256 |
| --- | --- |
| Final inventory (generation 7) | `7c67a0e32b58a9877b2e3d820d22695804449315e2fa8e381ec17ec7c03e82ba` |
| Installed publisher | `cf6a1dbab6b76605724047e4ce7c53b818c5be7ff72ac97c61c10a00d3ac726d` |
| Installed renderer | `43509b446b0423f212a0dab0c9fd867c7cf591a7eedd132845efd679b9e38cba` |
| Final managed route | `b435698ad8e280903b2ade00ce6935c6ea60d2fd511995c83b68b45f19328b19` |
| Native ingress public certificate | `851997f4b23c86fe6bd981e525c5f2bb22ec7bb2e9797a26518284c732624cac` |

The root-only helper and renderer are installed under `/usr/local/libexec/prichal/`. Almaz receipts, full configuration snapshots and reload payloads remain root-only under `/var/lib/prichal-managed-publisher/<revision>/`; immutable public trust is `/opt/server/management/config/caddy/certs/prichal-managed/<revision>/`. Operator bundles/metadata are `~/.local/state/prichal-managed-publisher/` on Almaz and `~/.local/state/prichal-staging-02-bootstrap/managed-publisher/` on Kvant. Local receipts are `.local/managed-publisher/`. Private configuration snapshots and credential custody must not be committed.

Validation passed: focused renderer/command race tests, nine publisher boundary tests, all 18 managed homelab boundary tests, and the full pinned smoke gate (56 Node checks, frontend build, Go vet and all Go race packages). The public protocol qualifier runs with `node deploy/homelab/qualify-managed-public.mjs --confirm-staging-02 --out NEW_PRIVATE_FILE`; output is exclusive. Hosted CI is separate. The native transport leaf still expires October 29 at 09:55:18 UTC; this unit adds no renewal authority.

This publisher intentionally supports initial publication of this one fixture only. A second endpoint, handle binding, inventory update or route replacement is refused. General publisher authority/update semantics, durable readiness/failure/retry state and DNS account/registrar custody evidence remain open in #7/#126. Existing public routes do not establish those contracts. Account/default-handle activation remains #8.

The Caddy behavior used here is documented in its [command-line validation reference](https://caddyserver.com/docs/command-line) and [configuration API/concurrency reference](https://caddyserver.com/docs/api). Fresh local tests verified the root POST/ETag behavior against the exact live image. The existing wildcard was inspected without applying a [Cloudflare tunnel configuration](https://developers.cloudflare.com/tunnel/features/locally-managed-tunnels/configuration-file/) change.

Subsequently, PR #208 merged as `310973f91706da548d4d6bfc5d809590452836bc`. The [durable public readiness qualification](managed-public-readiness.md) now records schema-41 ready state and the serving URL, with retained failure/retry receipts and no route change. This report's checking/no-serving-URL observations describe publication-time state. General publisher updates and ongoing renewal/withdrawal remain unfinished.

PR #209 subsequently merged durable readiness as `db94ee59ddfef98886525025bdfae74ffcf6b961`. The [inventory update qualification](managed-publisher-updates.md) extends this initial-only helper to complete endpoint inventories, explicit preceding revision and monotonic generations. The same fixture passed generation-8 update/rollback and generation-9 republication/replay. That report supersedes this report's helper hash, managed-route hash and initial-only restriction; the main Caddyfile and unrelated inputs remain unchanged. Handles, additional live placements and ongoing renewal/provider-outage qualification remain open.
