# PDS-30: Invite organization members with scoped management roles

[Gitea issue #30](https://git.subcult.tv/PatrickFanella/pds-ass/issues/30)

<!-- pds-ass-roadmap:30 -->

## What to build

An owner invites colleagues to manage permitted hosting tasks without sharing the owner's management login or PDS account password.

## Acceptance criteria

- [ ] Define owner, administrator and viewer permissions for deployment, domain, billing, support and export actions.
- [ ] Invite acceptance is expiring, single-use and tied to the intended verified management identity.
- [ ] Members see only their permitted organizations; revocation takes effect on subsequent requests.
- [ ] Prevent loss of the last owner and record ownership-transfer/member-change audit events.
- [ ] Keep hosting-management permissions separate from AT account signing and editorial delegation.

## Verification

Invite and revoke several identities, attempt role escalation and cross-organization reads, and verify a valid ownership handover.

## Blocked by

- #2: Sign in and create an owner-scoped organization
- #22: Give operators a tenant support and audit workspace

## Boundaries

No shared PDS passwords, automatic DID/email-based identity merging or implied organization publishing approvals.

## Planning reference

Phase: M5 - Additional access and portability. Priority: P2.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
