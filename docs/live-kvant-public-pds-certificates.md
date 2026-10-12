# Public Kvant pilot retained-key certificates — October 11–12, 2026

The fixed pilot now has a Kvant host operator that inspects its certificate expiry hourly and renews leaves within three days or the CA within thirty days. The CA signer and worker key stay at private `~/.local/state/prichal-kvant-public-pds/pki/`. Agent and ingress keys stay inside the guest. Only public CSRs/certificates and bounded status/identity receipts cross SSH. The operator is independent of the existing Dozor and private staging certificate timers.

## Installed scope

`renew-public-pds-certificates.py` is installed under the private host `bin/`, alongside the fixed public qualification harness and a link to pinned Node 24.15.0. User units `prichal-public-pds-certificates.service` and `.timer` require the existing VM and agent tunnel to be active. The timer inspects thirty seconds after activation and every hour; it does not start a paused VM. The private maintenance pause marker refuses renewal. During certificate activation, the operator stops the existing recovery timer/oneshot, then restores the timer only if it was active beforehand.

The guest root helper is `/usr/local/libexec/prichal-public-pds/certificate-guest.py`; its root-only receipts are `/var/lib/prichal-public-pds-certificates/<run>/`. It accepts only fixed check, prepare, activate, rollback-qualification and rollback-state requests. The signer is absent from the guest. Agent renewal uses a guest-generated public CSR whose retained public key must match both installed certificate copies. Ingress is self-signed with its existing guest key. Strict certificate profile, fixed identity, validity and runtime/storage/custody checks precede activation.

Almaz's root-owned `/usr/local/libexec/prichal-kvant-public-pds-trust.py` can update only `certs/prichal-kvant-public-pds.crt`, preserving the pinned public-key digest `acc683b7e2fb49fb4596f18d63847c75aae0ce9d4ea62626363343eb793a2d8d`. It receives only public certificates, permits at most two, and checks the fixed ingress subject/key/lifetime. It prepares overlapping preceding/new trust and validates/reloads the full Caddy configuration before guest activation. No route, other trust file or public identity is remapped. Root-only receipts live under `/var/lib/prichal-kvant-public-pds-trust/`.

Guest activation rewrites the existing ingress file inode because Caddy retains a single-file bind mount, then restarts only that edge container and agent service. It preserves native container, mount/custody, agent key and ingress public key. Host certificate copies are published only after successful guest activation and source-hash checks. Strict mTLS and the full public protocol probe verify the renewed consumer. The guest helper compares certificate subjects in RFC2253 format, which is stable across host and Ubuntu OpenSSL display defaults.

## Live qualification

Initial inspection passed without mutation. The first rollback attempt stopped before guest activation because of OpenSSL subject display differences. Retained guest certificates/runtime were independently compared with staging receipts; private mTLS and public checks passed, and Almaz trust was restored before the corrected run.

Forced rollback run `20261012T021854Z-7ceaeff9` activated candidate guest certificates, deliberately failed, restored preceding guest certificates and verified retained runtime. The coordinator then verified old mTLS health, restored Almaz trust through its unchanged-hash receipt, and passed public health/signup checks. `rollbackVerified=true`; that injected-failure receipt intentionally records `success=false`.

Forced renewal run `20261012T021913Z-afbdefaf` passed with `success=true`, `renewed=true`. The CA now expires January 10, 2027 at 02:19:14 UTC. Agent, worker and ingress certificates expire October 19, 2026 at 02:19:14 UTC. Native `f9459debe13d`, ingress `8c8c6ec4973b`, guest agent key, ingress public key, signer/worker keys and native custody were retained. Private mTLS and public health/metadata, empty repositories, closed XRPC/OAuth signup and WebSocket checks passed.

The first naturally scheduled inspection, `20261012T021956Z-37e58ed5`, passed with `success=true`, `renewed=false`; no fresh certificate was replaced. Both recovery and certificate timers remain enabled.

All 34 homelab tests passed, including seven new real OpenSSL CSR/profile, foreign-key/public-material, renewal-threshold and rollback tests. The full pinned smoke gate exited zero with 44 Node checks, frontend build, Go vet and race-enabled packages. Disposable PostgreSQL is stopped with its volume retained. systemd unit validation passed. Hosted CI remains a separate result.

## Receipts, failure and rollback

Host run receipts and public candidates are private under `~/.local/state/prichal-kvant-public-pds/renewals/`; `last-certificates.json` records success, renewal/rollback state, role expiry, check time and bounded error type. Inspect that receipt and the user service journal for operator failure. This is scheduled expiry inspection, not routed alert delivery. A subsequent [read-only status publisher and alert qualification](live-kvant-public-pds-alerts.md) connects receipts to existing Dozor monitoring; it preserves existing notification routes and mutes.

An uncertain guest activation response retains overlapping Almaz trust unless the fixed guest helper proves original certificate bytes and runtime custody. The coordinator never rolls trust back independently after successful guest activation; public failure retains new trust and records failure for inspection. Guest rollback refuses concurrent file changes. Staging and root-only copies remain available for scoped recovery. Never restore an active duplicate of this identity.

To pause only this lifecycle, disable/stop `prichal-public-pds-certificates.timer` and stop its service before maintenance. Keep the recovery timer's maintenance marker/stop procedure when stopping containers. Retain keys, current certificates and receipts. Do not restore stale certificates independently across hosts: compare later changes and use the verified guest/Almaz receipts together. The earlier encrypted checkpoint predates both new operator units and renewed public certificates; isolated restoration requires reviewed final units/trust refresh before exposure.

Naturally due renewal, routed expiry/heartbeat alert delivery, physical Kvant reboot and refreshed checkpoint restoration remain unverified. Account enrollment, federation and production management provisioning remain separate gates. Original NUC/Dozor PDS services and private staging management were preserved.
