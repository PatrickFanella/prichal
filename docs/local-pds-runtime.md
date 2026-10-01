# Disposable local PDS runtime

This fixture runs the official Bluesky PDS through Caddy on <http://localhost:2583>. It is separate from management PostgreSQL. It does not create a public identity or a user account.

```sh
./scripts/pds-local.sh up
./scripts/pds-local.sh check
./scripts/pds-local.sh qualify
```

`qualify` creates a one-use invitation, checks SQLite integrity, writes a temporary blob-volume canary, restarts the PDS, verifies persistence, stops the PDS to check the proxy's failure response, and starts it again. The canary is removed afterward. The unused invitation remains in this disposable fixture. The command restarts only the `pds-ass-runtime` PDS; run it when no other local work is using this fixture.

The manifest is [deploy/local/compose.yaml](../deploy/local/compose.yaml). The pinned upstream image resolved to PDS version `0.4.5036` in local verification on 2026-09-30. Caddy and PostgreSQL also use digest pins; changing a tag alone does not upgrade them. Requalify an intentional digest update.

## Isolation and limits

- The PDS has no published port. Caddy exposes only `127.0.0.1:2583`, permits `localhost` and `127.0.0.1` host headers, and proxies HTTP and WebSockets. Its admin API is disabled.
- PDS networking is internal to Docker, with no external network attachment. Its PLC URL points to a closed loopback port, crawler targets are empty, and handles use `.test`. Caddy has a separate ingress network for its loopback port; the PDS remains on the internal network.
- Generated JWT, admin and rotation secrets are retained in ignored `.local/pds.env` with owner-only file permissions. Do not print or commit it. Keep it with the retained volumes.
- The PDS has separate `pds-data` SQLite and `pds-blobs` volumes. Neither contains management data. Restart and `down` retain both.
- CPU, memory and PID caps are set in Compose: PDS 1 CPU, 768 MiB and 128 PIDs; ingress 0.25 CPU, 128 MiB and 64 PIDs. Container capabilities are dropped; ingress retains only the bind-service capability required by the upstream Caddy binary. Neither service mounts the Docker socket.
- Invites and upstream rate limits remain enabled. Blob upload size is capped at 5 MiB. No user accounts are created by qualification. Disk quotas and an enforceable per-tenant account cap remain unimplemented and must be resolved before shared hosting. Docker Desktop named volumes do not enforce a tenant disk quota.

The ingress uses local HTTP. Public DNS, ACME/TLS and live federation are separate gates; do not reuse this development manifest as a production deployment.

## Evidence limits

The health endpoint is an upstream process check. The qualification script separately reads actual SQLite integrity and verifies an invitation through the admin protocol. The blob canary proves volume persistence, not an authenticated upload or repository record. Storage exhaustion, corrupted database behavior and a complete identity/record/blob journey still need isolated fixtures. Do not mark issue #4 or M1 complete from these checks alone.

For local identity work, introduce a dedicated test PLC directory using the upstream identity implementation and a mail capture service before creating `.test` accounts. Keep fixtures on the internal network and disallow public `plc.directory` traffic. Do not create production identities in CI. For live qualification, select a designated staging PDS domain, approved email sender and disposable live account explicitly, then record DID/handle resolution, record/blob reads, stream events and downstream visibility separately.

## Stop and recover

```sh
./scripts/pds-local.sh down
```

This removes only fixture containers and networks, retaining its volumes. `up` reuses the same secrets and data. Do not remove the volumes or `.local/pds.env` independently. Public DNS, live host selection and cloud spend remain undecided.
