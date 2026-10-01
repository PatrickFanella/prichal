# PDS-10: Attach and verify a customer-owned domain handle

<!-- pds-ass-roadmap:10 -->

## What to build

An owner configures their AT account to use a domain they own, while the hosted PDS endpoint stays stable.

## Acceptance criteria

- [ ] Show exact TXT or supported well-known verification instructions without requiring registrar transfer.
- [ ] Authorize the account change against the correct organization and proven account control.
- [ ] Validate forward and reverse identity claims and reject ambiguous, conflicting or invalid domain records.
- [ ] Show pending DNS verification and bounded rechecks without provisioning a replacement PDS.
- [ ] Test removal/change of the handle independently from endpoint and storage lifecycle.

## Verification

Use a controlled domain to attach a handle; test unverified account control, conflicting DID, DNS delay and unchanged PDS endpoint.

## Blocked by

- PDS-08: Guide the owner through first-account enrollment

## Boundaries

BYO handle is distinct from BYO PDS endpoint, and custom DNS is outside the managed-domain provisioning timing claim.

## Planning reference

Phase: M1 - Private deployment alpha. Priority: P1.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
