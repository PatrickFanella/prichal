# PDS-17: Suspend and resume service through audited operator actions

[Gitea issue #17](https://git.subcult.tv/PatrickFanella/pds-ass/issues/17)

<!-- pds-ass-roadmap:17 -->

## What to build

An authorized operator can suspend a tenant or account for an operational/abuse reason, explain its status and resume it without erasing retained data.

## Acceptance criteria

- [ ] Define distinct service and account suspension actions using supported PDS lifecycle semantics.
- [ ] Record actor, target, reason, prior state and outcome; require operator authorization on every action.
- [ ] Stop new enrollment and supported serving/writing as intended, preserving recovery and export eligibility.
- [ ] Provide a customer-visible status and a documented report/review workflow.
- [ ] Resumption reconciles actual runtime state and rejects conflicting lifecycle operations.

## Verification

Suspend a disposable tenant/account, prove the intended access restrictions and retained data, then resume it; reject owner-to-operator privilege escalation.

## Blocked by

- #14: Detect outages, failed backups and exhausted capacity
- #16: Enforce tenant isolation and runtime resource boundaries

## Boundaries

Suspension is reversible and separate from billing cancellation, scheduled deletion and downstream application moderation.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
