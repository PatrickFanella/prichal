# Populated local tenant isolation

The local drill uses two independently provisioned, confirmed accounts owned by Alice and Bob. It writes a retained record and image on each PDS, verifies the signed repository and matching subscription commit, and rejects wrong signing keys and tampered CAR bytes. Public protocol data remains public; this is distinct from management authorization and native write access.

```sh
node scripts/qualify-tenant-isolation.mjs .local/owner-enrollment/ALICE_JOURNEY_UUID .local/owner-enrollment/BOB_JOURNEY_UUID
```

The command accepts only private local journey paths and exact UUID localhost endpoints. It validates Docker labels, per-instance volume names, private networks, no native published ports, CPU/memory/process controls and dropped capabilities before mutations. Native tokens reach only the exact local TLS fixtures. A foreign token must fail with the pinned runtime's `InvalidToken` error, not merely an unspecified failed request.

Direct PDS-to-neighbor and link-local metadata connections must fail. The drill then abruptly kills Bob's fixed, validated tenant container, checks Alice's login and signed data while Bob is stopped, and starts the same Bob container in cleanup. Restored Bob data, credential/certificate hashes and both container IDs must match. A qualification lock prevents overlapping drills. A failed or interrupted run requires operator inspection; it never deletes retained databases or volumes.

The actual 2026-10-01 local drill passed two populated tenants, signed repositories, media, stream commits, owner boundaries, native token rejection, private volume/network checks, blocked direct neighbor/metadata access, neighbor continuity and data retention after the abrupt kill. Reports and public record identifiers stay in `.local/tenant-isolation/UUID/`; account credentials stay in their private journey files.

The checks establish configured CPU, memory and process ceilings. They do not yet establish behavior under sustained CPU pressure, OOM or aggregate disk exhaustion. The one-account admission guard and per-upload five-MiB limit do not bound total disk use. Storage quotas, disk-pressure admission and customer-readable limit behavior remain required.

The shared local PLC/mail fixture is a trusted development adapter connected to both private networks. Its unauthenticated captured-mail interfaces are unsuitable for customer isolation. This drill does not qualify that fixture as production identity or email infrastructure. The internal-network policy also does not yet provide a production egress path to public identity and federation services. Live metadata/host-admin isolation, threat review, independent hosts and realistic noisy-neighbor testing remain open for issue #16.
