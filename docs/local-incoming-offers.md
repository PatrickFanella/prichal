# Paging incoming collaboration offers

`GET /api/team/incoming/invitations/page` and `/transfers/page` return separate 50-offer pages for the signed-in management identity. Existing `/api/team/incoming` remains compatible with its 30-of-each bound. The browser uses the new routes for first/next pages and refresh. No list grants permission or accepts an offer.

Every invitation query checks exact current issuer/subject, pending state, expiry and current inviter ownership. Every transfer query checks pending state, expiry, current offering ownership and current active administrator membership. Transfer acceptance separately rechecks empty-organization and billing eligibility. Metadata contains only offer/organization reference, expiry and invitation role; no email, issuer/subject, keys or native authority. CLI credentials remain excluded.

Continuations preserve exact timestamp/UUID order and bind the management identity and offer kind. Strict bounded queries reject invalid, repeated, unknown and cross-identity/kind cursors. Pages are not frozen snapshots. First-page refresh incorporates new offers or changing eligibility. Expiry uses one statement timestamp and creates no write/audit.

The browser replaces one bounded page, cancels stale reads, clears failed results and focuses explicit counts/errors. Refresh replaces both current lists and discards delayed older responses. Reviewing acceptance uses the existing owner-scoped retained intent and an unchecked acknowledgement. Paging resets acknowledgement; a retained request permits offer reads while another mutation remains blocked. No acceptance is automatic.

## Local qualification

Full smoke passed Node checks, frontend build and Go vet/race/PostgreSQL tests. Disposable fixtures traverse 117 offers of each kind over three pages, including exact timestamp ties. Expiry, revocation, identity mismatch, owner changes and loss of administrator membership between pages remove ineligible offers. Tests cover cursor scope/bounds, changed recipient identity, compatible legacy limits, CLI exclusion and disabled mode. An initial fixture mixed PostgreSQL UUID/text parameter types; explicit UUID conversion corrected the seed. Optional golangci-lint remains unavailable.

Actual read-only drill `2d5ec057-768a-4ec8-8b9c-258894dcd14d` completed at 2026-10-01T18:02:03Z. Both retained identities have zero pending invitations or transfers. Repeats and legacy projections matched; malformed/cross-identity cursors and CLI credentials were denied. Audit/job counts, both native repositories/media, containers and protected custody were unchanged. Nonempty API pagination is qualified by disposable PostgreSQL tests, not these retained lists.

The actual browser keyboard reload focused the empty ownership-offer count with a visible outline. Temporary read-only browser responses exercised 50 synthetic offers then one final offer for both pagers, preserving count focus after pager removal. Reviewing a synthetic ownership offer focused the retained-review heading with acknowledgement unchecked and confirmation disabled; paging reset acknowledgement. The request was cleared through the existing reviewed UI and no mutation request was sent. Original fetch was restored and actual empty lists reloaded. No source offer was created.

A separate delayed synthetic invitation response was discarded after current-access refresh. Five sizes had no document/offer overflow: 320×568, 375×812, 812×375, 768×1024 and 1440×900. At 18:05:03–05Z, default feature disablement cleared both lists, focused their errors and cleared busy state. Refresh hid collaboration while ordinary owner controls remained available. Private metadata remains in `.local/incoming-offers-browser.json`.

T3 snapshots remain intermittent: receipt text capture succeeded, while this offer snapshot failed. Assistive screen-reader, delegated hosting/domain permissions, populated ownership transfer and live qualification remain open. Source API is on schema 24 with collaboration and other fixture permissions disabled; admission is open. No host operation, container/volume deletion or recovery-copy removal occurred.
