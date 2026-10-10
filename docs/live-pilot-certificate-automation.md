# Disposable pilot certificate automation

PR #139 is merged at `765a30184c8c158046e494fad6a9db017896f86e`. On October 9 CDT / October 10 UTC, the fixed Dozor/Almaz renewal procedure gained a Kvant coordinator, expiry telemetry and existing-stack Prometheus alerts. The management API/job processes receive no recovery keys, private host identities or Docker access. The live node-agent binary remains `1d64932`; the existing NUC PDS and management release are unchanged.

## Operator and monitoring placement

| Owner | Installed component | Behavior |
| --- | --- | --- |
| Kvant | `prichal-pilot-certificates.timer` and service | Inspect hourly; renew when the earliest certificate has three days or less remaining. |
| Dozor | `certificate-tools/certificate-host.py` | Stage the fixed pilot, verify retained keys/custody, activate only its certificate files and restart only its edge/agent; provide verified rollback receipts. |
| Almaz | Root-owned `/usr/local/libexec/prichal-pilot-trust.py` | Accept only bounded public certificate requests for the exact UUID, retained ingress public key and dedicated trust file. Validate/reload full Caddy configuration. |
| Dozor | `prichal-pilot-certificate-metrics.timer` | Export certificate expiry and operator heartbeat every fifteen minutes through the existing Node Exporter textfile collector. |

Installed scripts match content bundle `f705f86880e3127d` by SHA-256 readback on Kvant, Dozor and Almaz. Kvant's `release` and Dozor's `certificate-tools` links select retained versioned directories; preceding directories remain available. Almaz's fixed helper is root-owned and hash-checked.

Kvant's durable operator root is `~/.local/state/prichal-pilot-certificates/`, outside the T3 worktree. The service uses existing operator SSH aliases with batch mode, strict host-key verification, bounded commands/timeouts and no SSH multiplexing. It holds a nonblocking local lock. Remote commands and paths are fixed; incoming data contains only a run ID and public certificates. No new SSH key or general management control endpoint was created. Both timer owners have lingering enabled; an actual host reboot remains untested.

Dozor stages each unique run under `/srv/apps/prichal/pilot/certificate-runs/`. Private prior/candidate identities, source hashes, activation-started and verified activation/rollback receipts remain on Dozor. Almaz's root-only receipts are under `/var/lib/prichal-pilot-trust/`. Its helper pins the ingress public-key DER SHA-256 `284f770cb482ec7400981d1507daa5bad427e4d8d45cf90a84d63825d7b49b66` and rejects private material, unrelated keys, subjects, hostnames or excessive validity. It changes only the dedicated public trust asset, leaving Caddy's main/site files byte-identical.

Expiry/heartbeat metrics are written atomically to `/var/lib/prometheus/node-exporter/prichal-pilot/certificates.prom`. The existing root-owned textfile directory contains only a dedicated symlink to this public metrics file; private pilot directories remain mode 0700. Node Exporter needs no access to certificates or keys, no new listener and no service recreation. Prometheus's existing `dozor-node` target supplies `host="dozor"`.

## Transaction and failure boundaries

The coordinator inspects authenticated agent health and current expiry before deciding whether renewal is due. Fresh certificates are inspected without signing, restarting or changing trust. When due, Dozor stages candidates with retained keys. Almaz installs overlapping active/candidate public ingress trust and validates/reloads it before Dozor installs the candidates. Dozor rechecks source hashes, roles, key agreement, loopback hostname, validity and fixed runtime custody before changing files. It retains the ingress file inode for Caddy's existing bind mount, flushes writes and restarts only the pilot edge and agent.

A local activation failure restores preceding certificate files and verifies health and retained runtime identity before recording rollback. The coordinator restores Almaz trust only when Dozor returns an explicit rollback receipt and currently matches the preceding certificate bytes. A lost transport response or unverified state retains overlapping trust and requires inspection; removing candidate trust independently could strand a successfully renewed edge. A later public-health failure similarly retains new trust and records operator failure. Concurrent file changes are preserved rather than overwritten by rollback. SIGTERM is handled during host activation; hard interruption/partial writes remain a manual recovery boundary.

The source [coordinator tests](../deploy/pilot/test_certificate_coordinate.py) cover lost activation responses, verified rollback, public failure after activation and fresh-certificate inspection. The operator supports explicit `--qualify-renewal` and `--qualify-rollback` options for the disposable pilot; the installed timer uses neither. These options are qualification actions with real certificate activation/restarts and must not be used as ordinary status commands.

## Live qualification

The latest forced successful renewal is `20261010T032747Z-773dd5aa`. It passed staging, overlapping Almaz trust, full Caddy validation/reload, Dozor activation, authenticated agent health and public health/signup checks. The CA now expires January 8, 2027 at 03:27:48 UTC; agent, worker and ingress expire October 17, 2026 at 03:27:48 UTC. Native/edge/guard container IDs and mounts, native credential state, policy receipt and signing keys were preserved. Native enrollment still reports the original DID/handle and confirmed email.

The preceding injected post-activation failure, `20261010T032709Z-6d1e7f22`, completed verified local and Almaz trust rollback before returning failure. An unrelated ingress key was also refused without changing the installed trust asset. Earlier preparation/qualification receipts are retained privately, including an initial failed qualification attempt. Active certificates and runtime custody were checked before the rerun; runtime comparisons now normalize mount ordering. No native data was replaced.

Both installed timers subsequently completed natural runs. Kvant's first natural run `20261010T032826Z-15516c48` returned success with `renewed=false`: the seven-day leaves were outside the three-day window. Dozor's first natural metrics run exited 0. This proves timer execution and fresh-state checks; a naturally due renewal has not occurred yet.

Prometheus ingests all four role expiry series and the successful operator heartbeat. Its four dedicated rules are loaded, healthy and inactive. Rules warn within two days of expiry, on failed certificate parsing, missing/stale telemetry, and failed or stale operator heartbeats. Rule fixtures passed approaching-expiry, fresh-validity, stale-heartbeat and missing-telemetry cases. Full production configuration validation and reload passed; unrelated alert rules and routes were preserved. Notification delivery has not been tested.

## Verification, recovery and remaining gates

Run `python -m unittest discover -s deploy/pilot -p 'test_certificate_*.py'` for the coordinator's five failure-boundary checks. Use the installed Prometheus image's `promtool test rules` against `certificate-alerts.test.yml` in a disconnected disposable process for rule fixtures. The project smoke gate remains `./scripts/dev.sh smoke` with its separate disposable PostgreSQL; its 44 Node checks, web build, vet and race-enabled Go packages all passed and are separate from live TLS evidence.

For automation rollback, first disable only the Kvant coordinator timer and Dozor metrics timer. Preserve current identities and all run receipts; disabling a timer does not revert an active certificate. Compare source/runtime hashes before selecting a preceding operator release. Restore only the checked certificate/trust transaction whose current hashes match its receipt, then verify authenticated agent and public PDS/signup behavior. Do not replay the dated manual activation helpers over later identities. Leave native containers, named volumes, policy, existing NUC service and production management owners intact.

The dedicated Prometheus rule file and added rule-list entry have a root-only preceding configuration/validation receipt under `/srv/apps/prichal/pilot/certificate-monitoring-20261010/`. Removing those rules requires preserving later configuration edits, complete validation and reload. The collector symlink and public metrics directory belong only to this pilot; retain private certificate directories separately.

Naturally due renewal, recovery after hard interruption, reboot reconciliation, sustained monitoring and notification delivery remain open. The independent encrypted snapshot retains preceding TLS validity and needs a fresh validity check before a later public recovery. Automated backup/key redundancy, authenticated browser login, full public host-loss cutover and production management integration remain separate gates. No public rollout or existing-SUBCULT identity migration is claimed.
