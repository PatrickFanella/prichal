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

`./scripts/build-customer-cli.sh` prepares five binaries and `SHA256SUMS` in ignored `.local/cli-build/`. In the initial October 1 qualification, all five checksum digests matched. The macOS ARM64 binary completed the actual authorization/deployment drill. Linux ARM64 executed version/help/completion in a separate disposable container using the already-pinned local PostgreSQL image, with no network, a read-only binary mount and dropped capabilities. At that initial stage, macOS AMD64, Linux AMD64 and Windows AMD64 were cross-compiled only; later runtime observations follow below. No release was published.

Twenty macOS launches measured about 6 ms median startup. The first cold launch took about 541 ms, so the under-50-ms cold-start target is not qualified. Responsive browser verification remains open: the collaborative preview reported page metadata but its snapshot failed. Windows credential ACL behavior, platform runtime installation tests, signing and published integrity metadata remain release gates. CLI first-account setup remains in the browser. Public DNS/TLS/email/federation and the 15-minute live onboarding target still require selected infrastructure. Issue #27 remains open.

## Repeated offline runtime checks

Build fresh local binaries with `./scripts/build-customer-cli.sh`, then run `node scripts/qualify-cli-runtimes.mjs` on an Apple Silicon macOS workstation. The qualifier checks all five checksum entries and runs eight offline cases on native macOS ARM64, macOS AMD64 through Rosetta, and Linux ARM64 in the existing pinned image. It pulls no image, publishes no release and performs no account authorization or API request. Each configuration directory is separate and private; missing-sign-in checks leave it empty. Linux runs without networking, with a read-only binary/root, dropped capabilities and bounded resources. Existing containers and volume names must match afterward.

Drill `f5740f1d-b4b4-40b7-b6e8-fd48983a66aa` completed at 2026-10-01T20:22:34Z. Version, root/deploy help, bash/zsh/fish completions, unknown-command exit 2 and missing-sign-in exit 1 passed on all three runtimes. Error cases had empty stdout and actionable stderr. All five checksums matched before and after. No credentials were created, no binary changed, and existing containers/volumes matched the baseline. Private results remain in `.local/cli-runtimes/`.

That October 1 drill added Rosetta execution evidence to the earlier cross-build evidence. It did not qualify Intel hardware, Linux AMD64, Windows execution/credential ACLs, installation, signing, published delivery or live onboarding. The first translated version launch took 1,879 ms; subsequent translated cases took 14–17 ms. Linux timings include container startup. The cold-start target remains unqualified.

Completion checks validate generated text, not installation into a shell session. Shell integration remains unqualified. The final full smoke gate passed after adding the qualifier and evidence.

## Translated macOS authorization and scoped reads

`node scripts/qualify-translated-cli-auth.mjs` runs the freshly built/checksummed macOS AMD64 binary through Rosetta against only `http://localhost:5173` and the disposable Alice/Bob issuer. Each receives a separate private configuration. The fixture browser reviews/approves requests started by the qualifier itself. It reads existing organizations and Bob's native server; it never submits a deployment. Individual revocation/logout removes only the qualifier's credentials. Source management work, native inventory/custody and binary hash must match afterward. Authentication activity intentionally changes.

Drill `cba280d7-3676-4139-a61a-2f2037beaf34` completed at 2026-10-01T20:27:20Z. Both device approvals and owner-scoped reads passed. Alice could not read Bob's server or revoke his credential. Bob's own revocation blocked further reads; logout removed his credential file even after server revocation. Alice's logout also revoked/removed her credential. Neither token appeared in CLI output. Both qualifier credential IDs were absent from active credentials afterward. No server deployment, worker pause or native mutation occurred, and source work/native custody/binary matched the baseline. Private report remains under `.local/translated-cli-auth/`.

This proves the local translated binary's authorization/read/revocation path. The earlier macOS ARM64 interruption/retry drill remains the deployment evidence. Intel hardware, other platform authorization, published installation and live onboarding remain open. No universal platform qualification follows from this drill.


## Native Linux AMD64 offline runtime — October 20, 2026

The offline checker now also runs on a native Linux AMD64 host:

```sh
./scripts/build-customer-cli.sh
node scripts/qualify-cli-runtimes.mjs
```

Use the repository's pinned Go and Node toolchains. The build prepares all five binaries and `SHA256SUMS` locally; the checker validates every digest before and after execution. On Linux AMD64 it executes only that native binary and needs no Docker daemon or image. The existing Apple Silicon matrix retains its native macOS ARM64, Rosetta macOS AMD64 and pinned-container Linux ARM64 paths. Unsupported hosts still refuse execution. Reports list the current host, binary hashes and platforms not exercised by that run. A Linux run explicitly records no Docker resource comparison, rather than inferring unchanged containers from an omitted comparison.

Drill `d3ffca9d-f553-47f6-a996-aab6cb41393b` completed on Linux x64 at `2026-10-20T10:14:20.112Z`, using fresh binaries from CLI source at `79f53f03aa97e55447e0c3b654992652e4c1cbb5`. All eight cases passed: version, root/deploy help, bash/zsh/fish completions, unknown-command exit 2 and missing-sign-in exit 1. Error cases emitted actionable stderr and empty stdout. The private 0700 configuration directory stayed empty. The native binary's SHA-256 was `00f25c3b1d80e350726bb066bcb056fd88d66b9ffab19e84607e15ab88fea6ad`; all five binary digests and the checksum file stayed unchanged.

The same run passed with a Docker shim that would record and refuse any invocation; its marker remained absent. No account authorization, API request, tenant allocation or release publication occurred. Private evidence is `.local/cli-runtimes/d3ffca9d-f553-47f6-a996-aab6cb41393b/report.json` and `.local/issue-audit/native-linux-cli-runtime.json`, the latter retaining the source revision, qualifier/build-script hashes and shim check. Local build artifacts remain in `.local/cli-build/`; this checkout had no prior build directory.

This closes the native Linux AMD64 offline-execution gap for this build. It does not combine earlier macOS/Linux ARM64 observations into a current-build pass. Those platforms, Windows runtime/credential ACLs, signing, published integrity and installation checks, authorized API journeys and live onboarding remain separate release gates. No cold-start performance target or installation claim follows from eight command executions. Issue #27 remains open.


## Native Linux loopback protocol and interruption — October 20, 2026

With a checksum-verified native binary in `.local/cli-build/`, run:

```sh
PDS_QUALIFY_CLI_LOOPBACK=true node scripts/qualify-cli-loopback.mjs
```

The opt-in driver supports Linux AMD64 and binds two ephemeral HTTP listeners only on `127.0.0.1`. One simulates device/token and organization/deployment responses; the other detects an incorrectly followed redirect. All credentials, identities and operation data are synthetic. No real management service, browser approval, database, Docker daemon or PDS tenant is used. The fixture issues its token directly and does not test account authority, issuer policy or browser approval.

Final drill `99f85ae0-6cf4-41ba-affd-f50b8655050d` completed at `2026-10-20T10:47:51.828Z`. Twelve actual CLI processes passed 30 assertions. Starting with an empty configuration directory, synthetic device/token exchange returned safe JSON and wrote a private 0600 credential file under a 0700 directory. Public-mode and symlink credential files were refused before protected HTTP requests. Organization and operation output stripped injected unknown secret fields; the synthetic access token and device code were absent from every captured process output.

The driver sent a real SIGINT after the waiting CLI's first status poll. The executable handled it and exited **130**, rather than terminating through an unhandled signal. Its saved 0600 request file retained the original payload and key. A new process retried that key, received pending exit **3** and the same fixture deployment identity, and left the request bytes unchanged. Changed payload reuse exited **2** before another POST. The two same-key POSTs referred to one fixture operation. Explicitly simulated failed/ready status responses produced exits **4/0**. The redirect target received no request. A simulated revoked credential produced actionable denial; logout after that denial removed its credential file while retaining the saved request.

Both listeners and every owned child closed before the driver wrote a passing receipt. A separate SIGTERM probe interrupted the qualification driver while it owned a native child: the driver exited 1, wrote a failed receipt with verified cleanup, and independent `/proc` readback confirmed the original child exited. The public-mode test restores credential permissions even on failure. These cleanup checks do not require freeing or modifying a retained tenant.

Private evidence is `.local/cli-loopback/99f85ae0-6cf4-41ba-affd-f50b8655050d/report.json`, `.local/issue-audit/native-cli-loopback-final.log` and `.local/issue-audit/native-cli-loopback-interruption.json`. The driver reused the unchanged `00f25c3b1d80e350726bb066bcb056fd88d66b9ffab19e84607e15ab88fea6ad` binary from the native offline drill above, built from CLI source at `79f53f0`. No customer CLI Go source changed between that source and this qualification. The report retains the exact qualifier hash; checksum metadata and binary bytes matched afterward.

This qualifies native process, file and client-protocol behavior against a synthetic loopback fixture. It does not prove durable server work, real device approval/revocation, management authorization, native provisioning, published installation or other platforms. Issue #27 remains open.


## Private Bash and Zsh completion sessions — October 20, 2026

With Bash, Zsh, standard-library Python and the checksum-verified Linux AMD64 binary available:

```sh
PDS_QUALIFY_CLI_SHELLS=true python scripts/qualify-cli-shells.py
```

The opt-in qualifier generates the native binary's completion text, then registers it in fresh interactive Bash and Zsh PTYs. The sessions use private completion files, an isolated command path/configuration location and disabled personal startup/history settings. Zsh uses a private cache path with dump creation disabled. Nothing is installed into the user's shell profiles. Typed test lines receive a real Tab key, then a private editor widget captures and clears the buffer before it can execute a CLI command.

Drill `d1b9cdb8-d78e-41ec-a7c6-308154f31a66` completed at `2026-10-20T11:30:59Z` with Bash **5.3.15** and Zsh **5.9.2**. Four editor-buffer checks passed in each shell: `sta` completed to `status`, `--dep` to `--deployment`, `--idem` to `--idempotency-key`, and an unmatched prefix stayed unchanged. Both shells exited 0 and were reaped. The private Zsh dump remained absent; standard personal history/cache metadata, the retained native binary and checksum metadata matched afterward.

A separate fault probe changed one expected completion in a private copy of the qualifier. It exited 1 with a failed Bash-stage receipt, recorded the mismatch and still reaped the owned Bash PTY with exit 0. Independent before/after personal-history/cache metadata also matched. This verifies failure reporting and ordinary assertion cleanup; no stuck-shell or external-signal behavior is inferred from that probe.

Private evidence is `.local/cli-shells/d1b9cdb8-d78e-41ec-a7c6-308154f31a66/report.json`, its private PTY transcripts, and `.local/issue-audit/native-cli-shells-failure.json`. Receipts retain the exact qualifier and binary hashes. The unchanged binary is the same `00f25c3b1d80e350726bb066bcb056fd88d66b9ffab19e84607e15ab88fea6ad` artifact used above. No CLI Go source, account, management API, PDS tenant, persistent user profile or published release changed.

This closes the representative Bash/Zsh shell-session registration gap on this Linux host. Fish was unavailable and remains unqualified. These checks do not qualify every completion prefix, persistent user installation, other shell/OS versions or signed release installation. The wider #27 release and authorization gates remain open.
