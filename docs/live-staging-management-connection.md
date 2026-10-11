# Private Kvant management connection — October 11, 2026

The dedicated Kvant node is connected to a separate private management stack. Source commit `12bda879c9bfc5041095c4bae986b235fa56e0e0` supplies its API, worker, observer and disposable OIDC issuer. The immutable management image is `sha256:15685feeb202b34af17583c3bbb0d7e3142093be422462f0fc31d1d6ebdbd21f`. The frontend is installed outside T3 worktrees. Production management on Dozor, the Dozor public pilot and the original NUC PDS remain separate.

## Scope and access

The dashboard listens at `http://127.0.0.1:5177` on Kvant, API at loopback 8087 and disposable issuer at loopback 5558. Alice and Bob are synthetic qualification subjects, not production identities. A local, expiring entitlement grants one staging instance; no paid subscription or real account was enrolled. Existing local-mode database, listener and provisioning guards remain active.

Private runtime configuration, encryption/enrollment keys, operator subjects and artifacts live in `~/.local/state/prichal-staging-management/`, mode 0700. The separate registry remains `prichal-staging-registry-postgres-1` on loopback 35435. Management PostgreSQL is distinct from native PDS SQLite and blob volumes.

API mounts contain management keys and encrypted artifacts only. Worker and observer receive a dedicated directory containing the authorized worker client leaf/key and public CA. They receive neither the CA signing key nor VM SSH key. API has no node client credentials. No management container mounts a Docker socket. Containers have read-only roots, dropped capabilities, no-new-privileges and bounded CPU/memory. The restricted guest agent owns native Docker execution.

## Measured behavior

Authenticated observation records node capacity and invalidates freshness after a failed measurement. The cookie-authenticated operator endpoint exposes a bounded projection of node ID, region, state, measured resources, freshness and reserved/available slots. It excludes transport endpoints, credentials and key pins. Ordinary owners receive 404; CLI bearer credentials receive 403. The UI refreshes every fifteen seconds and clears stale rows after failed authorization or measurement.

Node `1ef77d45-c6d5-4bdb-b77b-ba1103b45a66` has three virtual CPUs, about 3.8 GiB usable RAM and one operator-authorized slot. Real API/worker placement created deployment `53a1f2d3-181b-4cf2-b242-2d35ca55c784` for the synthetic Alice organization `ea360a14-eccd-45c1-b637-400876bf9fe1`. It reached `ready`; repeating the request retained the deployment and exactly one placement. Bob cannot read Alice's deployment. The inventory now shows one reserved slot and zero available slots.

The native endpoint is private: `https://53a1f2d3-181b-4cf2-b242-2d35ca55c784.localhost:37409`. A loopback SSH tunnel exposes its guest edge. Explicit trust of its exported public certificate returned native health version `0.4.5036`. No global browser trust was installed. No accounts, public DNS, email or federation have been qualified for this instance.

Browser inspection showed the ready server and occupied node on desktop and a 375-pixel mobile viewport without horizontal overflow. This qualifies the private fixture dashboard, not production OIDC or public browser access.

## Certificate consumers

The staging operator now accepts only instance namespaces already placed on this registered node. Foreign or duplicate namespaces fail its custody check. Renewal atomically publishes public CA/worker certificates to the dedicated client directory. Node-agent clients reload authorized credential files per operation and fail closed if replacement credentials are invalid.

Forced renewal with the retained placement succeeded at 22:32:38 UTC. The running worker then applied API suspend/resume operations with stable ticket replay. Native container `ef37ab59e4cc` and edge container `51c5e82a2ad0` retained their IDs. This verifies credential consumption after renewal, not merely certificate publication. The node leaf expires October 12 at 22:32:38 UTC; the hourly inspection at 22:45:06 UTC correctly skipped fresh credentials. Naturally due renewal and independent expiry/heartbeat alerts remain separate gates.

## Verification and remaining gates

`./scripts/dev.sh smoke` passed with the pinned toolchains against separate disposable PostgreSQL: 44 Node checks, frontend build, Go vet and all race-enabled packages. Twelve homelab operator tests passed. Credential reload rejection/recovery, inventory authorization, stale capacity, occupied placement and measured-capacity observer fixtures passed. Node syntax checks and Compose validation passed. `scripts/qualify-staging-management.mjs` records authentication, owner isolation, ready placement and replay against these fixed private origins.

Issue #3 is closed following the bootstrap merge. This connection adds private management qualification; it does not claim public hosting completion. Public staging routing, production authentication/management integration, populated account recovery, scheduled backups, unattended native runtime recovery and physical host boot/cutover remain unqualified. The native PDS containers require authenticated reconciliation after a guest stop/start; their restart policy alone does not restore service.

## Updated independent checkpoint

A cleanly stopped guest and paused management writers produced a fresh checkpoint of the VM disk, registry logical dump, retained PKI/operator custody, management keys/artifacts/configuration, systemd units and immutable management image. The bounded staging maintenance window was 61.36 seconds. Restart and authenticated reconciliation retained native/edge container IDs; subsequent health returned `0.4.5036`, and the management qualification repeated successfully at 22:55:53 UTC with the same deployment and placement.

AES-256-GCM ciphertext is retained on Kvant and independently at Dozor's `/srv/recovery/kvant/prichal-staging/staging-connection-20261011.pvm`. Ciphertext SHA-256 is `7c6195a3f3f0f38b0716584537b64b2ab8a12882569718f65e4d0883fdfaab78`. The existing separate private recovery-key copies remain required. Retrieving the Dozor copy and authenticated decryption recovered the exact trusted plaintext digest. Generated plaintext archives, image export and registry dump were removed. This new checkpoint was not boot-restored; the earlier empty-node restore does not prove recovery of its new placement and management state.

Fresh original-service checks returned NUC version `0.4.5027` and Dozor pilot version `0.4.5036`; Kuznya's VM retained PID 930757. The disposable smoke database was stopped after checks, with its volume retained.

The connected-node checkpoint was subsequently boot-restored with an independent registry and management stack. Retained runtime IDs, node certificate/key pin, placement, service generation and owner isolation passed. See [the dated restore qualification](live-staging-connection-recovery.md) for the isolated procedure, stopped fixture state and remaining populated-account/public-cutover gates.
