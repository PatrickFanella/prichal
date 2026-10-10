# Disposable pilot certificate renewal

PR #138 merged at `84aea8bc1747daf19530c7dae25cfbc457bae125`. On October 9 CDT / October 10 UTC, the Dozor pilot renewed its dedicated CA, agent/worker leaves and private ingress certificate using their existing keys. This changes certificate validity and serials, not account identity or PDS signing keys. The live agent binary remains `1d64932`; management production integration remains closed.

## Active identities and evidence

The CA is valid until January 8, 2027 at 02:55:59 UTC. Agent, worker and ingress certificates are valid until October 17, 2026 at 02:55:59 UTC. Leaves retain seven-day lifetimes, and the ingress remains within the agent's existing eight-day maximum. The agent uses serverAuth with loopback IP/localhost names; the worker uses clientAuth. The ingress uses the exact pilot UUID localhost name. No key was generated or replaced, system trust was not changed, and certificate verification remains required.

The [operator staging tool](../deploy/pilot/stage-certificates.py) reads only the fixed disposable pilot paths on Dozor. It verifies private regular inputs, ownership, existing certificate/key agreement, issuer roles and ingress hostname before signing with retained keys. It creates a new private directory containing prior identities, candidate certificates and a receipt; it never installs candidates, restarts services or contacts the management API. A second staging run passed against the renewed identities. Reusing its existing directory failed without changing live identity hashes.

The activated candidate is retained under `/srv/apps/prichal/pilot/certificates-20261010/`, with mode-0700 directory and mode-0600 files. Its `before/` identities and staging/activation reports support guarded rollback. The replay candidate at `certificates-replay-20261010/` is retained but was never installed.

Only the pilot edge and agent were restarted. The activation completed in 1.17 seconds; this measures the operation, not an externally measured outage. The native container ID, data/blob mounts, edge container ID, PDS credential state, policy receipt and CA/agent/worker private keys matched their preceding receipts. Authenticated agent status returned healthy, enrollment remained verified with the unchanged DID/handle and confirmed email, and a client without a certificate was rejected at TLS.

Almaz retains the preceding and renewed public ingress certificates together in `/etc/caddy/certs/prichal-pilot-ingress.crt` inside Caddy. Both correspond to the retained ingress key. Complete Caddy configuration validation and forced graceful reload passed; main configuration and the imported Prichal site remained byte-identical. Almaz's private operation directory is `~/.local/state/prichal-pilot-edge/certificates-20261010/`, with previous public trust, candidate certificate and validation/reload receipts. No private key left Dozor for this renewal.

Native password login, confirmed email, retained record value/CID, blob hash and CAR hash also matched after renewal. Full pinned-toolchain smoke passed 44 Node checks, web build, Go vet and all Go race packages; Python syntax and diff checks passed. Canonical operations audit returned no integrity errors.

Fresh public checks returned pilot health 200 at `0.4.5036`, both XRPC and native OAuth signup 403, and the existing NUC health 200 at `0.4.5027`. Certificate renewal does not establish a new browser login, reboot recovery or public host-loss cutover.

## Bounded operator procedure

Run before current certificates expire. The staging tool deliberately refuses invalid existing identities; recovery after expiration needs a separate inspected procedure.

1. Inspect current agent/edge consumers, exact instance container labels and mounts, private identity permissions, certificate dates and matching key identities. Preserve current native credential/policy hashes without printing their values. Check retained receipts and subsequent changes before selecting any prior certificate.
2. Run `python deploy/pilot/stage-certificates.py ABSOLUTE_NEW_PRIVATE_DIRECTORY` on Dozor as the existing pilot operator. The parent must already be private and operator-owned. Never reuse a directory or transfer its private contents to a management/API process.
3. Export only the candidate public ingress certificate to Almaz. Retain preceding trust, install overlapping previous/candidate public trust at the existing dedicated file, then validate the entire Caddy configuration and gracefully reload it. Preserve subsequent route edits and hashes; restore preceding trust if validation/reload fails. Keep UUID TLS server name and public Host forwarding intact.
4. Recheck staged source hashes against live files and validate candidate certificate/key agreement and intended hostname/roles before installation. Retain all private keys. The edge uses a single-file read-only bind mount: preserve its TLS file inode when replacing its contents, flush/fsync, and restart only this edge so Caddy reads the new identity. Replace the agent's CA/leaf and worker leaf, then restart only the dedicated agent unit. Do not recreate the native PDS, volumes, guard or public route.
5. Check authenticated agent health, certificate-less TLS rejection, enrollment identity, native content and public health/signup. On failure, restore only the four changed certificate files from the checked preceding snapshot, preserving the ingress inode; restart the same edge/agent and repeat verification. Keep overlapping Almaz trust until rollback and current consumers are reconciled. Rollback identities are retained and validated; an actual rollback activation was not exercised in this slice.
6. Record active expiry dates and hashes, restart/readback results and unchanged native storage/identity. Retain private receipts and update canonical host evidence.

The fixed one-off activation runners remain in private host operation directories. Their paths and expected hashes are tied to this activation, so they must not be replayed blindly. The committed staging tool is the reusable source; cross-host activation still requires the guarded operator procedure.

## Remaining gates

Manual certificate renewal is qualified. Automated issuance, cross-host trust distribution/reload, expiry alerts and a natural scheduled renewal remain open. These short-lived leaves still require another renewal before October 17; renewing shortly after bootstrap demonstrated the procedure and only moved leaf expiry by about two hours. A successful one-off renewal is not unattended lifecycle support.

The independently encrypted snapshot captured the preceding certificates. It retains the same keys, but its restored TLS validity must be checked and renewed before any later public recovery. Backup scheduling, key redundancy, reboot reconciliation, authenticated browser login, full public cutover and production management integration remain open. Local smoke, canonical registry integrity and live TLS/public checks are separate evidence.


## Automation follow-up

The bounded cross-host coordinator and expiry monitoring are now installed and qualified through forced renewal, verified rollback and natural timer inspections. This supersedes the preceding unimplemented issuance/distribution/expiry-alert observations. A naturally due renewal and notification delivery remain unverified; see [automation evidence](live-pilot-certificate-automation.md).
