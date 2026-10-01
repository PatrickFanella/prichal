# PDS-28: Deploy on a verified customer-owned PDS endpoint

[Gitea issue #28](https://git.subcult.tv/PatrickFanella/pds-ass/issues/28)

<!-- pds-ass-roadmap:28 -->

## What to build

Before creating accounts, an owner chooses and proves control of a custom PDS hostname and receives a working supported endpoint on that domain.

## Acceptance criteria

- [ ] Explain handle versus endpoint and check required separation from application/OAuth domains.
- [ ] Validate domain ownership, required DNS records and certificate eligibility before activation.
- [ ] Reject domain conflicts, unverified hosts and arbitrary DNS proxy/redirect destinations.
- [ ] Persist the endpoint choice before account enrollment and preserve it through node recovery.
- [ ] Prevent a silent hostname change for populated PDSs; route those requests to an explicit migration plan.

## Verification

Deploy with a controlled custom domain, test DNS delay/conflict and certificate failure, then restore while retaining its endpoint.

## Blocked by

- #7: Expose stable managed PDS hostnames with HTTPS
- #11: Protect credentials and prove identity-key recovery

## Boundaries

DNS completion is outside the managed-domain timing target; no automatic registrar takeover or endpoint rename on live identities.

## Planning reference

Phase: M5 - Additional access and portability. Priority: P2.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).

Schema-30 exclusive hostname reservations now have local competing-proof, atomic audit/release/expiry, recovery-hold and actual local-namespace refusal evidence. See [local claims](../local-exclusive-endpoint-claims.md). Positive public reservation, browser review, populated-claim recovery and routing/TLS/activation remain open.

Local browser claim review, scoped receipts and exact-ticket retry are implemented. See [workspace evidence](../local-endpoint-claim-workspace.md); public DNS, populated-claim recovery and activation remain open.

Disposable populated claim snapshot, held refusal and exact receipt replay are verified. See [recovery evidence](../local-populated-claim-recovery.md). Independent recovery, public DNS and activation remain open.
