# PDS-27: Deploy through a downloadable Go CLI

<!-- pds-ass-roadmap:27 -->

## What to build

A customer authenticates in the browser and uses a downloadable CLI to deploy, inspect and resume the same PDS operations supported by the management API.

## Acceptance criteria

- [ ] Implement safe browser/device authorization with short-lived grants and revocable scoped CLI credentials.
- [ ] Provide deploy/list/status commands with flags, interactive prompts and machine-readable output.
- [ ] Interrupting the CLI does not cancel server work; retries use the same deployment identity.
- [ ] Return clear exit codes for pending/failure states and avoid secret values in output/history.
- [ ] Publish tested platform binaries and integrity metadata; select Homebrew/npm wrappers only if useful after the binary flow works.

## Verification

Install a release binary on supported systems, authorize, deploy and reconnect after interruption; test expired grants, noninteractive flags and JSON output.

## Blocked by

- PDS-02: Sign in and create an owner-scoped organization
- PDS-05: Submit and resume a durable deployment request
- PDS-09: Prove signed records, media and downstream federation

## Boundaries

The CLI is optional; npm delivery is not required and fleet/provider credentials never ship to customers.

## Planning reference

Phase: M5 - Additional access and portability. Priority: P2.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
