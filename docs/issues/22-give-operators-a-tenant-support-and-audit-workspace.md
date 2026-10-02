# PDS-22: Give operators a tenant support and audit workspace

[Gitea issue #22](https://git.subcult.tv/PatrickFanella/pds-ass/issues/22)

<!-- pds-ass-roadmap:22 -->

## What to build

An operator can find an affected organization, inspect safe deployment/billing/recovery evidence and perform only authorized support actions with an audit trail.

## Acceptance criteria

- [ ] Search by safe organization/instance references and view runtime, billing, domain and incident state.
- [ ] Separate operator authority from customer ownership and redact credential/private-record content.
- [ ] Link existing retry, suspend/resume and recovery procedures to their operation history.
- [ ] Record support actions and reason; support inspection does not silently impersonate the customer.
- [ ] Expose customers' own activity history without leaking another organization's events.

## Verification

Use restricted operator and owner sessions to inspect a failed deployment and audited retry; test forbidden actions and sensitive-field redaction.

## Blocked by

- #17: Suspend and resume service through audited operator actions
- #18: Gate paid deployment with subscription entitlements

## Boundaries

This workspace uses already qualified lifecycle actions; it does not expose a raw terminal or Docker administration.

## Planning reference

Phase: M3 - Paid pilot. Priority: P1.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).

Original-operator exact lifecycle request receipts now have audited scope/recovery and rollback verification. See [receipt evidence](../local-operator-lifecycle-receipts.md). [Browser recovery](../local-operator-lifecycle-workspace.md) now has local contract and intercepted keyboard/failure qualification. Actual operator policy and remaining visual/responsive checks remain open.
