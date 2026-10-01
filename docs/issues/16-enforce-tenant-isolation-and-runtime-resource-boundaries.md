# PDS-16: Enforce tenant isolation and runtime resource boundaries

<!-- pds-ass-roadmap:16 -->

## What to build

Two independently provisioned organizations can operate their PDSs without reading each other's state or exhausting the entire host through ordinary supported requests.

## Acceptance criteria

- [ ] Isolate runtime credentials, volumes, networks, blob namespaces and management authorization.
- [ ] Apply CPU/memory/storage/account controls with customer-readable limit behavior.
- [ ] Keep database and agent interfaces private; block access to cloud metadata, host admin interfaces and other tenants.
- [ ] Validate host/instance/path identifiers instead of accepting arbitrary shell commands or images.
- [ ] Document shared-host failure boundaries and any limits that still require operator intervention.

## Verification

Run two populated tenants and attempt cross-tenant API, storage and network access; exercise resource limits and a tenant crash while observing the neighbor.

## Blocked by

- PDS-06: Provision a real PDS through the restricted host agent
- PDS-07: Expose stable managed PDS hostnames with HTTPS
- PDS-11: Protect credentials and prove identity-key recovery

## Boundaries

Container isolation is not dedicated-machine isolation; security review must assess the actual supported runtime and threat model.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
