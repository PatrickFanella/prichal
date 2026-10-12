# Kvant public pilot automatic recovery — October 11, 2026

The fixed public pilot now has a host operator timer that recovers its retained native and ingress containers after guest startup. It uses the existing private mTLS worker certificate and key, without Docker access, SSH execution, signer access, management enrollment or account creation.

`deploy/homelab/reconcile-public-pds.py` is installed at `~/.local/state/prichal-kvant-public-pds/bin/`. The user units `prichal-public-pds-reconcile.service` and `.timer` run it 20 seconds after timer activation and every calendar minute. The service requires the VM and both local tunnels to already be active; it does not start a paused VM. Calendar scheduling retries even after a prerequisite failure. SSH tunnel supervision remains responsible for reconnecting after guest reboot.

Before any mutation, the operator GET checks exact instance `9f148c55-da61-486b-97f4-50df26b65893`, endpoint `https://prichal-staging-pds.subcult.tv`, known runtime state and boolean health. The agent independently validates retained container/storage/network/ingress policy. A missing namespace, foreign identity, suspended state, invalid TLS or malformed response fails before PUT. Healthy runtime requires only GET. An unhealthy retained runtime receives the fixed PUT with `{}`; credentials, identity and signup policy remain in the restricted agent. Redirects and proxy environment variables are excluded.

The private `last-reconciliation.json` receipt records identity/status, whether reconciliation ran, health and check time. Errors record only their type. A just-started native runtime can be unhealthy during warmup; that invocation exits nonzero and a later calendar invocation observes health. This is recovery/status evidence, not an alert-delivery system.

For container maintenance, create mode-0600 `~/.local/state/prichal-kvant-public-pds/reconciliation-paused` before stopping containers, or stop the timer. An owned private pause marker prevents all transport/key reads. Remove it to resume recovery. Stopping the timer alone does not stop an invocation already running; stop the service too before runtime maintenance. For VM maintenance, preserve the existing tunnel-before-VM stop order. Do not use this operator to create a replacement deployment or restore a cloned identity alongside the live pilot.

## Verification

Seven operator tests cover missing-runtime refusal, retained-runtime recovery, healthy read-only checks, pause, wrong identity/suspended state, private-key symlink refusal and redirect refusal. All 27 homelab tests passed. systemd unit validation passed. The full pinned `./scripts/dev.sh smoke` exited 0 with 44 Node checks, frontend build, Go vet and race-enabled packages against separate disposable PostgreSQL. That database was stopped afterward with its volume retained.

The live VM stop/start test retained native container `f9459debe13d` and ingress `8c8c6ec4973b`. After restarting its host tunnels, a naturally triggered timer invocation reconciled both without a manual PUT. Public health, exact OAuth issuer/service descriptor, zero repositories, XRPC/OAuth signup 403 and WebSocket 101 passed again. The installed pause marker also produced `paused=true`, `reconciled=false` and was removed after verification.

A second in-guest `systemctl reboot` recovered through existing SSH supervision and the calendar operator without restarting either host tunnel manually. The 02:05 UTC invocation reconciled the retained containers, and the 02:06 UTC invocation confirmed healthy state with only GET. Container IDs stayed unchanged and the public protocol probe passed. This proves guest reboot recovery while Kvant and Almaz remain running.

The original private staging VM and Kuznya retained PIDs 1994020 and 930757. Existing NUC and Dozor PDS health returned 0.4.5027 and 0.4.5036. No existing account identity changed.

## Limits and rollback

Physical Kvant reboot is untested. Certificate renewal/expiry alerting remains a separate gate: this timer cannot recover expired credentials or replace trust. Agent/operator certificates still expire October 18, 2026 at 23:53:46 UTC; private ingress expires at 23:56:05 UTC. Account enrollment, downstream federation, populated restore and production management provisioning remain open.

To roll back only this automation, disable/stop `prichal-public-pds-reconcile.timer`, stop its oneshot service, and retain the script, mTLS material and receipts. Manual authenticated reconciliation remains available. Preserve the VM, both local tunnels, edge route, volumes and all earlier PDS services. The earlier encrypted checkpoint predates these units; install the reviewed final recovery/tunnel units during an isolated restore rather than activating a second live identity.
