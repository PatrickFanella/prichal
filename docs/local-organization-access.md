# Local management collaboration

The collaboration API is a disabled-by-default rehearsal. Set `PDS_LOCAL_ORGANIZATION_ACCESS=true` only for the disposable local management API; production startup rejects enabling it. A normal restart disables its routes while retaining invitation, membership and audit evidence. Existing owner APIs and CLI scope are unchanged. See [the permission contract](local-organization-access-design.md).

An owner invites an already verified management identity by its identity reference and selects administrator or viewer. The invitation snapshots the target's exact issuer and subject, expires after 24 hours and can be accepted only by that target's authenticated browser session. Stable invitation retries preserve one invitation/audit. An accepted receipt can be replayed after revocation but cannot regrant membership.

Members can read a small shared server-status projection. Administrators can rename the organization. The owner controls invitation, role change and revocation. Subsequent requests recheck current access. Member invitation management has a separate disabled-by-default [local operator opt-in](local-member-invitations.md#administrator-invitation-permission--october-19). First-account enrollment, identity signing, domain mutation, billing, private export, support authority and hosting exit retain their existing owner/native permissions. No email, arbitrary terminal or Docker capability is exposed.

Ownership handover is a separate offer with a 24-hour expiry and explicit recipient acceptance. It requires an active administrator and an organization with no deployment or billing binding, rechecked under the organization lock at acceptance. The offer does not change ownership. Acceptance changes the single owner atomically with its audit, invalidates pending former-owner invitations and grants no implicit access to the former owner. Revocation or demotion cancels pending offers to that member. Populated hosting handover requires separate custody and billing work.

## API

All `/api/team/` routes require browser sessions; mutations require origin and CSRF. Routes list permitted organizations and incoming offers, return safe shared status and expose owner-only member/audit history. Owner invitation submission uses a stable UUID request. Acceptance addresses the existing invitation/offer ID. Membership and rename mutations record only effective changes. The public API has no deletion or native account delegation route.

Shared instances and the management membership/audit summaries are bounded to 100 records; current active members are included before retained revoked history. The compatible organization array route is bounded to 100; the browser now uses [paged permitted organizations](local-organization-pages.md). Each incoming-offer list is still bounded to 30. Exact organization references remain usable for other organizations. `hasMore` marks truncated server lists and `membersTruncated` marks membership truncation. Full listing pagination is later work. The [local browser workspace](local-collaboration-workspace.md) adds reviewed, retained actions on top of this API.

## Qualification

Full smoke passed frontend build, Node checks and Go vet/race/PostgreSQL tests. Coverage includes exact target identity, expiry/replacement, eight concurrent stable invitations, single-use acceptance, immediate revocation/demotion, role escalation denial, last-owner protection, separate transfer consent, population rechecks, recovery holds, bounded private projection and audit-failure rollback. Test-fixture UUID/text casts and required fixture hostnames were corrected before the gate passed; no source data was affected.

Actual drill `38d7984c-e379-4fad-80b8-b92f9775860a` completed at 2026-10-01T15:26:53Z. Bob accepted a viewer invitation to Alice's hosting organization and read safe server status. Private exports, member administration and role escalation were denied. Missing CSRF failed. Owner revocation took effect immediately, and accepted-invitation replay did not restore access. That source membership remains revoked.

A new empty organization exercised administrator rename and owner-only invitation controls. Alice's ownership offer left her as owner until Bob separately accepted. Repeated acceptance preserved one audit. Bob became the sole owner; Alice lost access. The empty organization and all audit evidence remain retained. Native signed records/media, container identities and protected credential/TLS/admission files were unchanged.

Management recovery `cbc79c44-2096-4503-b138-90e2aa1af9ca` matched all 43 schema-23 tables and sequence positions, including two invitations, two retained membership rows, one accepted transfer and eight access audits. Copied authentication was revoked and held worker startup refused. Exact host reconciliation and dedicated-queue replay preserved copied default jobs, source rows/sequences and native lifecycle/admission custody. Source writers were paused for capture/replay and then resumed with collaboration, exit scheduling, fixture billing and operator authority disabled.

Broader browser qualification, complete listing pagination, delegated administrator hosting/domain actions, populated ownership handover, outbound mail and live identity/provider qualification remain open for #30. No milestone is closed by this local rehearsal.

Owner access audits are also available through [paged organization history](local-collaboration-history.md). Each page rechecks current ownership. The existing management summary remains bounded; other collaboration lists still need pagination.
