# PDS-26: Pass the public-launch gate and open bounded signup

[Gitea issue #26](https://git.subcult.tv/PatrickFanella/pds-ass/issues/26)

<!-- pds-ass-roadmap:26 -->

## What to build

The team reviews a complete evidence packet, opens bounded public signup and can stop new enrollment without interrupting existing customer recovery paths.

## Acceptance criteria

- [ ] Verify the managed-domain deployment/account/federation journey, security/isolation results and both restore drills on release artifacts.
- [ ] Confirm current plans, support terms, retention/exit behavior, abuse contact and advertised timing match measured capabilities.
- [ ] Set signup/capacity controls and an immediate operator mechanism to close new deployments.
- [ ] List unresolved limitations and accountable owners; any failed critical gate keeps public signup closed.
- [ ] Record the release versions and observe the first controlled production deployments.

## Verification

Rehearse signup opening/closing, a normal new deployment, capacity exhaustion and a support incident; retain a signed-off release checklist with evidence links.

## Blocked by

- #23: Run an invited pilot and measure the deployment target
- #24: Finish accessible browser onboarding and lifecycle guidance
- #25: Qualify independent production deployment and incident recovery

## Boundaries

This issue does not grant an SLA, unlimited signup, arbitrary public-key/identity recovery powers or unmeasured region/capacity claims.

## Planning reference

Phase: M4 - Public launch. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).

## Local control evidence

[Management-owner admission](../local-owner-admission.md) adds audited pause/reopen of new verified management subjects while preserving existing-owner sign-in. It is separate from deployment/enrollment admission and issued native invitations. Local close/reopen and a fresh schema-27 recovery checkpoint passed. Production operator administration, designated live infrastructure, public launch evidence and sign-off remain open; this does not open public signup.
