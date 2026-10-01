# PDS-01: Run the standalone local stack and CI smoke journey

<!-- pds-ass-roadmap:01 -->

## What to build

A contributor can start a Go management API, React interface and disposable PostgreSQL, then open the interface and see database-backed service readiness.

## Acceptance criteria

- [ ] Document and pin supported toolchains, dependency locks and local startup commands.
- [ ] A versioned migration initializes the management schema; incompatible schema versions fail clearly.
- [ ] The browser calls the API and displays readiness without exposing configuration secrets.
- [ ] Gitea CI builds both applications and runs a clean-database smoke journey without cloud credentials.

## Verification

Start from a clean checkout, open the browser, stop PostgreSQL and verify readiness changes; run the same build and smoke gates in CI.

## Blocked by

None. This issue can start immediately.

## Boundaries

No deployment, billing, existing application imports, production cloud creation or live mail is needed.

## Planning reference

Phase: M0 - Runnable foundation. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
