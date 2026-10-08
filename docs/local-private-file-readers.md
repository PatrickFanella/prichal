# Private file readers on native Linux

Issue [#125](https://git.subcult.tv/subculture-collective/prichal/issues/125). The agent writes its TLS identity and policies as owner-only files. Caddy and the enrollment guard now run as the agent's numeric effective UID/GID, so native Linux Docker can read these bind mounts without changing host permissions.

Caddy listens on container port 8443. The reserved external loopback port and advertised PDS endpoint stay the same. Its writable configuration and data directories live under `/tmp`, avoiding the image's root-owned defaults. The pinned Caddy executable carries a `NET_BIND_SERVICE` file capability; its existing Docker capability allowance must remain for the executable to start. Other capabilities remain dropped, `no-new-privileges` remains enabled, and the guard has no added capabilities and a read-only root filesystem. Runtime validation rejects a changed reader identity or changed writable paths. The Caddyfile must remain a private regular file.

## Existing local instances

An old edge or guard fails the new runtime validation. Stop the local worker and agent before preparing an upgrade:

```sh
node scripts/prepare-local-private-readers.mjs INSTANCE_UUID
PDS_AGENT_LOCAL_PROTOCOL_FIXTURES=true ./scripts/agent-local.sh run
# In another terminal:
./scripts/agent-local.sh ensure INSTANCE_UUID
```

The helper accepts only a labeled instance using the isolated identity fixture and a localhost endpoint. It inspects both readers before changing them, saves their private inspection receipt, stops and renames them with `-before-private-readers`, and retains the Caddyfile with the same suffix. It does not stop or replace the native PDS, change its volumes, or change credentials or TLS identity. Reconciliation creates the new readers and Caddyfile. Preserve the agent's UID/GID across restarts; changing it requires a separate custody review.

For rollback, stop the worker and agent, stop and retain the replacement readers, restore the prior Caddyfile at its original path, and restore the prior container names. Run the previous agent release against the retained files. Do not restore a database or delete accounts for a reader rollback.

## October 8 qualification

On Kvant, native Linux Docker, a fresh disposable management database reached schema 35. Deployment `7f839e95-77fa-4c29-9419-abb1078caa29` ran a complete first-account journey, then the member invitation journey with a two-member ceiling. Reports are in ignored `.local/owner-enrollment/0a956da6-f32d-4789-8387-aa9c4ff8a62d/report.json` and `.local/member-invitations/3486a829-4e9e-4fad-9c8b-9fd8874a7efd/report.json`.

Checks passed for invitation replay and recovery, foreign-owner refusal, native creation/login, first-account admission closure, captured email confirmation, single-use member codes, ceiling enforcement, withdrawal, accepted member identity, preserved owner status and owner export with a member present. The reader upgrade then retained the populated PDS and the credential/TLS hashes; owner login passed after both readers were replaced.

The first provisioning trial failed while the Caddy image's required file capability was omitted. A qualification-only database transaction resumed the same deployment and River job after that correction; its failed events and native storage were retained. This is operator-assisted recovery in a disposable database, not a supported production retry workflow or an uninterrupted deployment measurement.

On Soyuz, Docker Desktop, isolated containers running as UID/GID `501:20` read the private enrollment policies, started the same pinned Caddy image, served trusted TLS and rejected an unknown Host with HTTP 421. Existing PDS containers and volumes were preserved. This checks the changed file-reader contract on Desktop; it does not repeat the complete owner/member journey there.

These drills use local identities and captured mail. Public DNS, public TLS, inbox delivery and downstream federation remain separate gates.
