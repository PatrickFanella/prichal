# Local first-account enrollment

The restricted agent can explicitly enable the isolated protocol fixtures with `PDS_AGENT_LOCAL_PROTOCOL_FIXTURES=true`. This uses the fixed local PLC and captured `.invalid` SMTP transport. Live identity, email delivery and federation remain unavailable. Default agent mode keeps identity creation disabled.

A dedicated guard shares only the tenant's internal network. It mounts a private enrollment directory read-only, has a read-only root filesystem and fixed resource limits, and holds no PDS administrator credential. Managed Caddy ingress checks both native account-creation routes against this guard. A private, current, tenant-bound lease is required for admission. Missing, malformed, expired, future or overlong leases fail closed, as does a stopped guard. The native PDS has no published host port that could bypass admission. Capacity accounting includes the guard's memory and CPU reservation.

The empty qualification fixture can be prepared with `node scripts/prepare-local-identity.mjs INSTANCE_UUID` after stopping the local agent. The script refuses populated or identity-enabled PDSs, records private rollback metadata, stops the preceding PDS and ingress, archives their containers and preserves volumes, credentials, endpoint and TLS identity. Start the agent in isolated identity mode and reconcile the same UUID. The original containers remain stopped for rollback; no source volume is removed.

`node scripts/qualify-enrollment-ingress.mjs INSTANCE_UUID` verifies the admission boundary and stable identity. Publication is checked from inside the isolated guard because Docker Desktop bind mounts can briefly lag host atomic file replacement. The drill restarts the same guard in cleanup and waits for its listener before declaring recovery.

The owner-authorized invitation broker, management status, native first-account journey and bounded invitation recovery remain the next work. Do not open a lease manually for customer enrollment. An approved sender, public certificates and designated live identities are still required before staging acceptance.

## Private broker qualification

The restricted agent's fixed enrollment request accepts only an instance UUID and ticket UUID through authenticated worker transport. Administrator calls execute in the already owned PDS runtime, where its administrator key remains. Output contains only a one-use code, expiration and public account status. Fixed helper output is bounded and checked; stderr is discarded as a generic failure.

A replacement closes admission, confirms publication inside the guard, drains the PDS process and recounts all actor rows before issuing another invitation. Pending signup requests finish or terminate before a replacement code is minted. Prior unused codes are revoked. Replaying a current ticket keeps its original code and expiration; replaying an expired ticket leaves it expired. A completed actor closes admission. The guard also checks native public repository inventory before admitting signup, so the first-account ceiling does not depend on a management worker refresh.

`node scripts/qualify-enrollment-broker.mjs INSTANCE_UUID` exercises the private local broker without completing an account. Codes are retained only in private `.local/enrollment-broker/` files and never printed. The drill verifies replay, replacement revocation, expiration and absence of administrator credentials. It leaves admission expired. This transport is not a customer API; owner authorization, bounded recovery and encrypted management invitation storage are the next layer.
