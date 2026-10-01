# Paged organization access history

The local collaboration workspace now loads all retained access audits through bounded pages rather than displaying only the latest 100. The existing management response remains compatible; its audit summary is still bounded. The new browser history reads `GET /api/team/organizations/{id}/audit` on request and does not create a job, access audit or host operation.

Every page requires current organization ownership within a read-only database snapshot. Rename-only administrators and viewers cannot read this owner history. An ownership change removes the former owner's access to later requests. The returned metadata includes event IDs, management identity references, action, role and time. It includes no email, native credentials, private native content or raw operation message.

Pages contain up to 50 records, ordered by descending numeric event ID. IDs remain strings in JSON and JavaScript, preserving bigint precision. The opaque cursor binds the organization and an exact positive int64 event ID. Malformed, noncanonical, oversized, repeated and unknown query inputs are rejected. A cursor grants no access, and this history is not a frozen multi-page snapshot. Newly recorded events appear when loading the latest page again.

The browser cancels superseded requests and discards results after unmount or organization changes. A successful load focuses its announced page count. A failed load focuses its error message. Access denial clears cached items. Loading never submits a management action.

## Local qualification

Full smoke passed. Disposable PostgreSQL tests traverse 157 events with IDs beyond JavaScript's exact integer range over four pages. A separate 117-event test traverses digit boundaries in numeric order. Tests cover owner changes, administrator denial, read-only behavior, cursor/query bounds, disabled routes and CLI exclusion.

Actual read-only drill `3c20cfa0-58c0-475d-b6b8-f55d832366d5` completed at 2026-10-01T16:59:52Z. Bob read eight events from the retained browser collaboration organization. Stable replay matched; former owner Alice, malformed/cross-organization cursors and CLI scope were denied. Access audit/job counts, both signed native repositories/media, containers and protected custody files remained unchanged. The fixture has one page; larger-page browser navigation remains unqualified.

The actual browser loaded these eight events through keyboard controls, showed a visible focus outline and focused the page count. DOM checks found no document overflow or offscreen history elements at 320×568, 375×812, 812×375, 768×1024 and 1440×900. A 375×812 screenshot was reviewed. Disabling collaboration cleared cached history, focused its error message and cleared busy state; refreshing access then hid the collaboration panel while the owner workspace stayed available. Private evidence remains in `.local/collaboration-audit-browser.json` and `.local/collaboration-audit-browser.png`.

The browser exposed a new query ordering error: the displayed `id::text` alias caused lexicographic ordering, placing event 9 ahead of 16. The query now orders the qualified numeric column, and the digit-boundary regression test covers this failure. An API startup race stopped one read-only attempt before qualification; a ready API completed the corrected drill. No source management mutation was required.

Organization, instance, membership, invitation-receipt and incoming-offer pagination remain open. Assistive screen-reader testing, populated handover, delegated hosting/domain work and production collaboration remain separate gates. Collaboration remains disabled by default.
