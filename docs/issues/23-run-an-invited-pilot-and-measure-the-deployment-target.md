# PDS-23: Run an invited pilot and measure the deployment target

<!-- pds-ass-roadmap:23 -->

## What to build

Invited customers complete deployment and first-account use while the team records actual completion time, recurring costs, support work and recovery evidence.

## Acceptance criteria

- [ ] Run at least 20 documented fresh managed-domain deployment attempts in the stated staging/pilot conditions and report all failures/retries.
- [ ] Report median, tail timing, success rate and both system provisioning and end-to-end onboarding intervals.
- [ ] Define exactly which external DNS/migration steps are outside the default path; do not present benchmark results as a guarantee.
- [ ] Invite a small bounded paid cohort only after recovery, isolation, entitlement and exit gates pass.
- [ ] Record cloud/storage/mail invoices, operator time, repeat use and customer problems; make a continue/change decision.

## Verification

Publish a secret-free pilot evidence report with raw attempt identifiers/timestamps, cost/support measurements and at least one verified recovery rehearsal.

## Blocked by

- PDS-09: Prove signed records, media and downstream federation
- PDS-10: Attach and verify a customer-owned domain handle
- PDS-14: Detect outages, failed backups and exhausted capacity
- PDS-15: Upgrade a PDS with staged rollout and safe recovery
- PDS-16: Enforce tenant isolation and runtime resource boundaries
- PDS-18: Gate paid deployment with subscription entitlements
- PDS-19: Show usage and enforce plan limits without surprise loss
- PDS-20: Add capacity and drain a hosting node safely
- PDS-21: Export data and cancel hosting with a safe exit window
- PDS-22: Give operators a tenant support and audit workspace

## Boundaries

Pilot participation and customer payment need actual consent; no claims of demand, production capacity or 15-minute performance before evidence exists.

## Planning reference

Phase: M3 - Paid pilot. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
