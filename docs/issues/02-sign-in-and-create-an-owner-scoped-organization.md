# PDS-02: Sign in and create an owner-scoped organization

<!-- pds-ass-roadmap:02 -->

## What to build

A customer signs in through configured OIDC, creates an organization and sees only that organization in the browser.

## Acceptance criteria

- [ ] Choose and document the production OIDC provider; local tests use an isolated test issuer.
- [ ] Validate state, nonce, issuer, audience and callback destinations; browser sessions use secure cookie settings.
- [ ] Identity keys use issuer and subject; matching email or DID never merges users.
- [ ] Organization ownership gates API access, and sign-out/revocation invalidates management sessions.
- [ ] Two owners cannot enumerate or modify each other's organization.

## Verification

Exercise successful login, rejected callback replay, wrong issuer/audience, logout and cross-organization access through browser and API tests.

## Blocked by

- PDS-01: Run the standalone local stack and CI smoke journey

## Boundaries

Management identity is independent of PDS login; multi-member organization roles are a later extension.

## Planning reference

Phase: M0 - Runnable foundation. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
