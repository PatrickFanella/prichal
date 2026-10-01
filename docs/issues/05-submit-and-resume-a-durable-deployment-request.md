# PDS-05: Submit and resume a durable deployment request

[Gitea issue #5](https://git.subcult.tv/PatrickFanella/pds-ass/issues/5)

<!-- pds-ass-roadmap:05 -->

## What to build

An owner names a server in the browser, submits a deployment request and can return later to the same durable operation and progress history.

## Acceptance criteria

- [ ] Persist deployment intent and a River job in one PostgreSQL transaction.
- [ ] Validate name, supported region and organization-scoped idempotency key.
- [ ] A test provisioner exercises queued/configuring/checking/ready/failed states and bounded retries.
- [ ] Closing the browser or restarting the worker does not lose progress or create another instance.
- [ ] Errors are customer-readable and secret-free; only authorized owners can read operation status.

## Verification

Replay an identical request, conflict on different parameters, crash the test worker and reopen the browser; verify one intent and one resulting instance.

## Blocked by

- #2: Sign in and create an owner-scoped organization

## Boundaries

This slice uses a test provisioner; real host execution follows without changing the customer contract.

## Planning reference

Phase: M0 - Runnable foundation. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
