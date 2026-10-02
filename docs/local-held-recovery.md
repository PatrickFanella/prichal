# Local recovery of a held native account

A native account takedown must survive restoration. The local held recovery qualifier applies an audited account hold, invokes fresh-volume managed recovery, then resumes the original through a separate audited account action. It preserves the distinct service and account states.

```sh
node scripts/qualify-held-recovery.mjs \
  .local/owner-enrollment/BOB_JOURNEY_UUID
```

The inner managed recovery command also accepts `--held-account` when the source already has a confirmed account hold. It requires a running service and an applied account lease for the matching native DID. It does not create a source marker or perform an ordinary restored write while held.

Cold capture fences source SQLite, blobs and the shared local identity/mail fixture. Authenticated decryption restores fresh volumes and protected host metadata. The restored native account must reject public login and repository serving. A fixed operator helper verifies signed repository data and every referenced blob through native admin export. The recovered PLC signer performs rotation and rollback only in the cloned local directory. Restored writers stop before the source starts, and the source account remains held until the separate operator action resumes it. Source native inventory, identity directory and protected metadata must match.

Recovery fixtures now select two unused `/24` subnets within `10.252.0.0/16` after checking existing Docker networks. This avoids exhausting Docker Desktop's default pool while retaining old recovery resources. It is a fixed local fixture pool, not a live networking policy. Concurrent address allocation still fails safely if Docker reports a conflict. No networks or volumes are pruned. See [Compose network IPAM](https://docs.docker.com/reference/compose-file/networks/).

Compose dollar escaping preserves the guard's JavaScript and protected runtime values. The drill checks rendered configuration and the actual created container command/environment. This prevents `${instance}` in the guard program from being interpreted as a Compose variable. Private subprocess failure logs remain ignored under `.local/`.

The outer qualifier waits for the original container's health check before requesting account resume. Native HTTP readiness alone can precede Docker health confirmation. Failed audited operations remain retained and require inspection; the qualifier does not rewrite their outcomes.

This remains an operator recovery fixture containing shared development identity/mail state. It is not a customer export. Service-held restore, independent backup custody, realistic datasets, user-deactivated accounts, key/certificate-loss recovery and live incident recovery remain open. Exact local evidence and failed attempts are recorded in [progress](progress/local-m1-alpha.md).

## Dynamic API readiness, October 1

The management API now checks current schema and recovery state on every readiness request. A hold introduced after startup, a missing ledger, incompatible schema or unavailable database produces generic HTTP 503 with `ready=false`. The existing startup recovery refusal remains in place. The response discloses no snapshot or host reconciliation details; the browser reports unconfirmed readiness instead of assuming the database is unreachable. The monitor's schema-only probe remains available for inspecting recovery conditions.

A disposable recovery database reproduced the former HTTP 200 during a runtime hold. The same handler now transitions 200 to 503 after the real hold and back to 200 after empty-host reconciliation; missing-ledger, incompatible-schema and closed-pool checks refuse readiness. Focused race checks and the full local smoke gate passed. No source recovery hold, account/session revocation, migration or native mutation was performed. Live health routing and independent recovery remain open.

The browser poll showed unavailable/unverified readiness with an intercepted 503, then returned to available after restoring native fetch. The fixture was removed and retained claim metadata remained absent. This was a browser response fixture, not a source recovery hold.
