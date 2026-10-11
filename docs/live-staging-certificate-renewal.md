# Private staging certificate renewal

The fixed Kvant staging node now has hourly certificate inspection and retained-key renewal. This extends [the private bootstrap](live-kvant-staging-node.md); it does not connect the production management application or create a public PDS tenant.

## Authority and publication

`node-admin renew --node UUID --out FILE` is private operator authority. It retains the existing local-mode and loopback `pds_local` database guards. There is no HTTP renewal route, bootstrap token or new identity. The registry locks the active node, verifies its retained certificate's signature, server role, endpoint hostname and public-key fingerprint, then issues a new server certificate for the same public key and endpoint. Unknown nodes, foreign state and key drift fail closed. The certificate has a 24-hour lifetime capped by its CA. Renewing an expired leaf is permitted only through this private operator path, allowing retained-key recovery.

The public certificate checkpoint is committed in the registry before its private output file is published. If file publication or transport fails, the active node may still serve its earlier certificate with the same registered key. Retain the receipt and retry the same operator; do not issue another bootstrap or replace the key. Registry and installed certificate serials are separate observations until publication/health succeeds.

Kvant's coordinator lives outside T3 at `~/.local/state/prichal-staging-control/renew-staging-certificates.py`. It uses a nonblocking lock, fixed node UUID and pinned SSH transport. It renews when the node has eight hours or less remaining, workload certificates have three days or less, or the CA has thirty days or less. It also corrects missing certificate profile extensions. `--force` is an operator qualification action, not a timer argument.

The CA, enrollment listener and worker retain their existing private keys and exact subjects. The renewed CA has a 90-day horizon; listener and worker leaves have seven-day horizons. CA/key usage, subject key identifiers, authority key identifiers, role EKUs and listener IP SAN are explicit. Retained-key CA refresh extends validity; it is not compromise-driven key rotation.

Only public CA/node certificates cross SSH. The root-owned guest publisher at `/usr/local/libexec/prichal/publish-certificate.py` accepts those two bounded fields. It refuses CA public-key replacement, node-key replacement, a foreign node subject, wrong certificate purpose/hostname and insufficient validity. It publishes to the fixed private agent paths, preserves ownership, fsyncs, and restarts only `prichal-staging-agent.service`. If local activation fails, it restores both prior certificate files and attempts to restart the previous agent. The node private key remains on the guest; the CA private key remains on Kvant.

The coordinator retains prior public certificate receipts under `renewals/`, records successful inspection/expiry in `last-renewal.json`, and records bounded failure type/time in `last-renewal-error.json`. Unexpected transport/health failure leaves receipts for inspection and makes the service fail. Do not infer rollback from a lost SSH response. Same-key trust is retained, so investigate and retry without copying private keys or recreating the node. Failure files are historical receipts; compare their timestamps to the latest success.

## Live qualification, October 11, 2026

Forced renewal completed at 22:00:53 UTC. The node ID remains `1ef77d45-c6d5-4bdb-b77b-ba1103b45a66`; its retained public-key hash is unchanged. The node leaf now expires October 12 at 22:00:54 UTC; listener/worker leaves expire October 18 at 22:00:53 UTC, and the retained-key CA expires January 9, 2027 at 22:00:53 UTC. These are observed expiries, not fixed service deadlines: subsequent successful renewals advance them.

Both the agent and private enrollment authority passed Python's default strict TLS verification after profile renewal. Authenticated capacity remained one slot on three CPUs/about 3.8 GiB RAM with no instance namespaces. A candidate signed by the real authority but carrying a different node public key was rejected by the publisher before either certificate file changed. The node key stayed unchanged.

`prichal-staging-certificates.timer` is enabled in Kvant's lingering user manager. Its first natural run at 22:03:09 UTC succeeded and skipped fresh credentials; its next observed run was scheduled for 18:03:09 Central. The service's hourly schedule is independent of this worktree. Naturally due renewal has not yet been observed. Journal/service status and the private scalar receipts are available; independent expiry/stale-heartbeat alerting and notification delivery remain unqualified.

Eleven local infrastructure tests passed, including due windows/profile repair, retained-key certificate profiles, symlink refusal and injected guest activation rollback. The Go enrollment regression verifies same-key/new-serial renewal, CA expiry bounds, valid trust, unchanged bootstrap rejection, unknown nodes and registry key drift refusal. Full smoke passed with 44 Node checks, frontend build, Go vet and all race-enabled packages. These local results are separate from hosted PR CI.

## Recovery and next gates

The encrypted October 11 bootstrap archive remains the original checkpoint. Its key identities are still valid recovery inputs, but its certificate files and operator binary predate this renewal. After an isolated restore, recover the current reviewed operator/CLI from Git and refresh retained-key certificates against the restored registry before enabling a management transport. Verify the stable node ID/key, absence of a second active identity and current TLS/expiry. Do not restore older registry or certificate state over subsequent live changes. Populated tenant recovery requires separate evidence.

Pause only `prichal-staging-certificates.timer` before intentional certificate maintenance and retain all private keys/receipts. To resume, inspect signer/registry/node identity and run the fixed coordinator, then re-enable the timer. Stopping the renewal timer does not stop the VM or agent; short-lived credentials will eventually expire.

Hosted issue #3 was closed after the bootstrap; that state is preserved. Follow-up qualification still includes integration beyond the separate registry, public staging domain selection, independent monitoring, scheduled/populated recovery and physical boot/cutover. The existing Dozor pilot and NUC PDS, their certificate automation and public routes remain outside this operator's scope. The operator deliberately refuses new instance namespaces until its custody checks are adapted and qualified for populated staging.
