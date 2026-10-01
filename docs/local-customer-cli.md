# Local customer CLI qualification

The customer binary is independent of the restricted node agent. It carries a management credential, never fleet credentials or Docker access. Build it with:

```sh
(cd backend && go build -o ../.local/bin/pds-ass ./cmd/pds-ass)
.local/bin/pds-ass login --api-url http://localhost:5173 --config-dir .local/my-cli
.local/bin/pds-ass organizations list --config-dir .local/my-cli
.local/bin/pds-ass deploy --organization ORGANIZATION_UUID --name my-server \
  --idempotency-key REQUEST_UUID --kind agent --wait --config-dir .local/my-cli
```

Replace both UUID placeholders with actual UUIDs. The local node currently has three occupied slots, so a new native deployment cannot be qualified without a separate capacity decision. `--kind test` runs a management simulation and reports that no PDS was deployed. Live provisioning remains disabled.

Sign-in prints a verification URL and human code to stderr. The user enters the code in the signed-in workspace, reviews its fixed permissions, then approves or denies access. The client waits at least five seconds between polls and increases that interval after `slow_down`. Grants expire after ten minutes and are consumed atomically. The resulting credential expires after 24 hours. No refresh token is issued. The JSON protocol follows the device-flow pattern; it is not a general OAuth server.

CLI credentials permit organization/deployment reads and deployment submissions. They cannot create organizations, request native invitations, access browser sessions or administer hosts. Bearer requests reject ambient cookies, Origin headers, mixed credentials and paths outside the fixed scope. Browser approvals and credential revocations require the owner's session, Origin and CSRF token. Owner isolation applies to every allowed API operation. Request/approval rate limits are stored in PostgreSQL; live proxy address handling needs qualification.

Private configuration directories require mode 0700 and files mode 0600. Symlinks and public credential files are rejected. Deployment payloads are persisted before submission; reusing a key with a different payload fails before a network request. HTTP redirects are not followed. Network requests have time and response-size bounds. The CLI requires verified HTTPS except for disposable loopback HTTP. Ctrl+C stops polling and leaves server-side work running. stdout is JSON, while instructions and progress go to stderr. See [command contract](cli-design.md) for flags, help, completions and exit codes.

## Actual local evidence

On 2026-10-01, qualification `afc742d3-fc37-40ce-b1e9-96888cc15c6d` completed at 10:33:50Z. Alice and Bob authorized separate CLI credentials through their local OIDC browser sessions. Bob could inspect his populated native placement; Alice could not inspect it. The credential could not create an organization. With only the local worker paused, a simulated deployment was submitted and the CLI wait interrupted with exit 130. A retry returned the same queued deployment. Restarting the worker completed it; a subsequent CLI status/list showed one ready deployment. Individual revocation rejected further CLI reads, another owner could not revoke it, and logout removed the private credentials. The signed native tenants were not mutated by this simulation.

Report: `.local/cli-qualification/afc742d3-fc37-40ce-b1e9-96888cc15c6d/report.json`. `scripts/qualify-customer-cli.mjs prepare` requires the operator to pause only this project's worker and set `PDS_LOCAL_WORKER_PAUSED=true`; `resume` runs after restarting it. These scripts target only the fixed local origin and disposable owners. A first attempt selected Bob's empty organization and stopped before submitting a deployment. Its credentials were revoked before the corrected drill.

Management recovery qualification `236cbd45-e887-401d-b10a-9709f92279a1` copied all 29 current schema 17 management/River tables and sequence positions. Its snapshot contained one active CLI credential, six device grants, 47 browser sessions and two approval-attempt records. The restored hold removed credentials, grants, browser sessions and login attempts. Worker startup refused the held copy. Reconciliation and a real lost-checkpoint River replay preserved all source row digests/sequences, signed native records/media, container identities and protected lifecycle/admission files. The source credential still worked afterward. A final all-device revocation invalidated that credential and a second approved, unconsumed grant. Reports remain under `.local/management-recovery/236cbd45-e887-401d-b10a-9709f92279a1/`.

`./scripts/dev.sh smoke` passed after these changes: frontend production build, Go vet, race-enabled clean-database integration tests and native fixture/recovery checks. Tests cover grant expiry/denial, concurrent single consumption, polling/rate limits, fixed scope, cookie/Origin rejection, revocation, copied authentication, persisted request identity, interruption, redirects and secret-free output.

## Distribution and remaining gates

`./scripts/build-customer-cli.sh` prepares five binaries and `SHA256SUMS` in ignored `.local/cli-build/`. All five checksum digests matched. The macOS ARM64 binary completed the actual authorization/deployment drill. Linux ARM64 executed version/help/completion in a separate disposable container using the already-pinned local PostgreSQL image, with no network, a read-only binary mount and dropped capabilities. macOS AMD64, Linux AMD64 and Windows AMD64 were cross-compiled only. No release was published.

Twenty macOS launches measured about 6 ms median startup. The first cold launch took about 541 ms, so the under-50-ms cold-start target is not qualified. Responsive browser verification remains open: the collaborative preview reported page metadata but its snapshot failed. Windows credential ACL behavior, platform runtime installation tests, signing and published integrity metadata remain release gates. CLI first-account setup remains in the browser. Public DNS/TLS/email/federation and the 15-minute live onboarding target still require selected infrastructure. Issue #27 remains open.
