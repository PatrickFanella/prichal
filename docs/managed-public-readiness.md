# Durable managed public readiness

This unit continues #7 after PR #208 merged as `310973f91706da548d4d6bfc5d809590452836bc`, with required head and merged-main checks passing. It adds an explicit worker-only `PDS_STAGING_MANAGED_PUBLIC_CHECKS=true` opt-in requiring production staging provisioning and managed native runtime. Existing private/local behavior and managed workers without this opt-in stay unchanged. Account/default-handle enrollment remains #8.

## Contract and qualification

Schema 41 retains each public-check result, its current bounded cycle and operator retries without changing existing endpoint/placement identity. A check requires the locked current generated reservation, placement/node key and fresh authenticated native managed health. Public DNS, validated HTTPS, health, service DID/OAuth endpoints, closed staging signup, WebSocket handshake and an unregistered generated-host rejection must all pass. Only the final transaction may set ready and the serving URL together with its receipt/event. Recovery holds around transport refuse checkpointing uncertain success.

The probe accepts deployment identity only. It derives both hostnames, refuses private/special-use/mixed DNS answers, pins validated public IPs, verifies system TLS trust/SNI, disables proxies/redirects and bounds headers, bodies and elapsed time. No DNS, certificate issuance, route mutation or host execution is available to it.

Build `./cmd/worker` and `./cmd/managed-public-retry` with the pinned Go toolchain into ignored operator storage. Load the designated management URL and existing worker-agent CA/certificate/key tuple through private custody. Set all four worker selections explicitly: `PDS_MODE=production`, `PDS_STAGING_PROVISIONING=true`, `PDS_STAGING_MANAGED_RUNTIME=true` and `PDS_STAGING_MANAGED_PUBLIC_CHECKS=true`. No new node-agent flag or API opt-in enables public checks. The retry command requires the fixed loopback `127.0.0.1:35437` registry, database `pds_local`, user `pds_staging` and explicit confirmation; invalid scope is refused before database work.

Failed public observations remain checking with explicit progress, retry after 15 seconds and stop at failed after three checks. Receipts and counts survive worker reconstruction. The operator-only `managed-public-retry --key UUID --confirm-staging-02` command is fixed to the second disposable registry/deployment/node. It permits only a bound completed public-failure cycle, retains hostname/placement/data, and atomically records one new cycle/event/job. Exact-key replay cannot enqueue another job. No customer retry/host administration route is added.

The live fixture was qualified with a process-local trust failure against the real public TLS endpoint, followed by same-identity retry and reconstruction after uncertain public success. The results below distinguish this worker trust failure from a DNS provider outage.

Public readiness records initial activation observations. Ongoing public monitoring, route withdrawal/renewal, general publisher updates and DNS-account custody remain separate gates. Existing rollback/custody limits in [staging publication](managed-staging-publication.md) and [conversion](managed-staging-conversion.md) still apply; do not restore an older private agent over managed state or force a schema downgrade.

## Native result — October 28, 2026

The same disposable staging-02 deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f` on node `60684f10-c6b2-4833-a0b8-fb9f96a7869b` now has a committed **ready** checkpoint and serving URL `https://pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv`. One native namespace, one placement, unchanged native/ingress IDs, credentials and volumes, stopped private predecessors and zero accounts were confirmed. The agent and public route were not replaced or reconfigured.

The normal-trust Go probe first passed against the actual public endpoint without persisting readiness. A separate worker process then used only the existing staging public CA as its temporary trust file and an empty private CA directory. Its pinned native mTLS connection stayed healthy; public Cloudflare TLS validation failed. The real River worker committed the first TLS failure, was stopped/reconstructed and completed the remaining checks. Cycle 1 ended failed with three TLS receipts and no serving URL. System trust, DNS and the public route stayed unchanged.

The operator retry key `cc016184-4e0f-4fec-9369-8d4f0f08faae` created cycle 2 and one new job; exact-key replay returned the identical receipt. With normal system trust in a new process, the worker obtained a successful real public probe and was interrupted before checkpointing it. The cycle still had zero attempts and the prior three failure receipts. A reconstructed worker committed one ready proof, event and URL together. Final state is cycle 2, attempt 1, four total checks and one retry. This proves retained failure/retry and uncertain-success replay for the same endpoint, not fresh API allocation, a provider outage or a new account/client login.

The public check verified current generated-host DNS, validated HTTPS, native `0.4.5036` health, exact service DID/OAuth issuer/authorization/token endpoints, closed native/OAuth signup, WebSocket acceptance and unregistered generated-host rejection. Receipts contain bounded outcome codes, public addresses, certificate digest/expiry, version and timestamps; response bodies, credentials and raw transport errors are not retained in customer progress.

Three retained management archives restored completely into separate allocator-owned copies:

| Snapshot | Tables / checks / retries | SHA256 |
| --- | --- | --- |
| Pre-upgrade, schema 40 | 66 / 0 / 0 | `5e41481f11c0cb98392c7d6f43e6e5cb7be96936400291cf22a3b8828665d7de` |
| Before checks, schema 41 | 69 / 0 / 0 | `2b51ef782e651b6fc95ae022671336189341c458df0e0def00a4b61e2da30e25` |
| After readiness, schema 41 | 69 / 4 / 1 | `94a3d119bd25927109246f8f1df2ab5addd86f869d89eff3ea3b5395c13e6583` |

All rows/logical hashes and sequences matched, including the retained conversion record. Raw/unmarked and held copies refused routing export and retained the public evidence unchanged. Sources stayed unchanged; exact disposable copies were dropped, and final readback found no recovery database. These are same-host management restores, not off-host or node-loss recovery.

## Verification, custody and pause

Focused and full pinned smoke gates passed with `GOFLAGS=-p=2`: 56 Node checks, frontend build, Go vet and all Go race packages. Controlled TLS/DNS tests cover mixed private answers, special-use addresses, system trust refusal, redirects, bounded responses, identity/signup failures and invalid WebSocket acceptance/token headers. PostgreSQL checks cover bounded failure, concurrent readiness/retry, exact-key replay, changed node keys, queue failure, unfinished jobs and late recovery holds/cancellation. Schema-40 upgrade preserves retained intent and invents no checks/readiness. All 18 managed homelab boundary tests passed. An initial focused database invocation used an incorrect local connection setup; rerunning with the repository's generated private environment passed. Hosted CI is separate.

Private scripts, candidate binaries, scope, reports and archives remain `.local/managed-readiness/` here and `~/.local/state/prichal-staging-02-bootstrap/managed-readiness/` on Kvant. The preceding schema-40 operator/recovery binaries are retained there; the agent binary remains unchanged. Candidate SHA256 values are `044a955c18c6a4e2d7adf777a468ab06acfa8b99bc5aaece89f6c2aab031d0c4` (worker test harness), `4ea764ae7a745dbcb0b8e9d6fc4d63dc132c9220c6525f05f204f15f04b1beb7` (retry command), `68a874ab3762d9c6d64acc5c427567a7b82a94e488e7b72e55e0a93db63ac961` (inventory command) and `a7fd4a8e85b05c12864012405c78042c57bfbb5195e3c1b033c4bf80163df7c0` (recovery command). These are worktree candidates, not a published release. Canonical infrastructure evidence is maintained in Machinum.

The qualification workers are stopped. Removing the public-check opt-in pauses future checks; it does not withdraw an already committed endpoint. Use the separate publisher's guarded rollback to withdraw its route after fencing workers. Keep schema 41 and all proof/history rows; older management binaries reject the newer schema, so never force a version downgrade. The private trust-failure files were applied only to the fault process and were not installed as system trust. The short-lived node transport certificate still expires October 29, 2026 at 09:55:18 UTC; unattended renewal remains unqualified.

#7/#126 remain open for general publisher inventory/update authority, supported DNS/provider failure qualification, ongoing renewal/withdrawal and DNS account/registrar custody. This initial activation does not qualify continuous availability, federation, browser sign-in or real account/default-handle enrollment.

PR #209 subsequently merged this unit as `db94ee59ddfef98886525025bdfae74ffcf6b961`, with required head/main checks passing. The [inventory update/rollback qualification](managed-publisher-updates.md) now records complete-inventory publisher support and live update/rollback/republication on the same fixture. Ready state, cycle/check/retry counts and native resources stayed unchanged; two fresh certificate observations were appended. Those later observations are outside the historical archive scope above. Additional live placements, supported DNS/provider outage, ongoing renewal/withdrawal and account custody remain separate gates.
