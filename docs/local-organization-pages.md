# Paging permitted organizations

`GET /api/team/organizations/page` returns up to 50 organizations currently owned by or shared with the signed-in management identity. The browser uses this route for its first/next-page controls. The existing `/api/team/organizations` array response stays compatible and bounded to 100.

The new page contains organization ID, name and current management role only. It does not include email, native identities, account credentials, billing or host configuration. The query rechecks ownership and active membership on every page. Revocation removes an organization, a role change updates its displayed role, and ownership takes precedence over membership. Opening a listed organization still requires its existing authorized status/management routes. CLI credentials remain excluded.

The continuation preserves exact creation timestamps and UUID ties. It binds the management identity, accepts only a bounded `after` query and grants no permission. Invalid, oversized, repeated, unknown or cross-identity cursors fail. Pages are not a frozen snapshot; refreshing the first page incorporates later changes.

The browser replaces one bounded page at a time, cancels superseded reads and clears selected organization details before changing pages. Loading never confirms or mutates a retained invitation, role or ownership action. Successful explicit loads focus the page count; failed loads focus the error. Access denial clears the displayed list. Incoming offers still have their existing limits.

## Local qualification

Full smoke passed Node checks, frontend build and Go vet/race/PostgreSQL tests. Tests traverse 118 organizations over three pages with exact timestamp ties and UUID ordering. Separate tests change membership, role and ownership between pages, verify cursor identity/bounds, retain the old array route and reject CLI access. Optional golangci-lint remains unavailable.

Actual read-only drill `0e232800-fd86-4e47-b75d-0c65a508785f` completed at 2026-10-01T17:15:13Z. Alice read two organizations and Bob read four. Stable repeats matched, old-route projections matched, malformed/cross-identity cursors and CLI credentials were rejected, and access audit/job counts plus both native repositories/media, containers and protected custody remained unchanged. These actual lists each contain one page.

The actual browser reloaded Bob's four organizations with keyboard controls, a visible focus outline and focused count. A temporary read-only browser response fixture exercised 50 synthetic organizations followed by one final organization, including removal of the pager without focus loss. A separate delayed synthetic response arrived after access refresh and was discarded. Both fixtures were removed, the original browser fetch restored and the real four-organization list reloaded. No source organization or management action was created by those fixtures; they qualify browser behavior, not live list data or cursor integration.

DOM checks found no document or collaboration overflow at 320×568, 375×812, 812×375, 768×1024 and 1440×900. Disabling collaboration cleared the displayed list and focused its error. Refresh then hid the panel while the ordinary owner workspace remained available. Evidence remains in `.local/organization-pages-browser.json`. T3 snapshots remain intermittent: the preceding history check captured a phone image, while this list check's text snapshot failed. Assistive screen-reader checks and production qualification remain open.

Instance, membership, invitation-receipt and incoming-offer pagination are separate remaining work. Collaboration remains disabled by default and cannot be enabled in production by its local fixture flag.
