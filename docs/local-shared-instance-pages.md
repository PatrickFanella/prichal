# Paging shared server status

`GET /api/team/organizations/{id}/instances/page` returns up to 50 shared server records for the current owner or active viewer/administrator. Each page checks current access in a read-only transaction. The projection contains deployment ID, name, recorded state and native/simulation kind only. It contains no native credentials, endpoints, billing, host configuration or private failure messages. CLI credentials remain excluded.

Exact timestamp/UUID continuation binds the management identity, organization and list kind. Invalid, oversized, repeated, unknown or cross-scope cursors fail. The cursor grants no permission. Pages are not a frozen snapshot; refreshing incorporates changes.

The browser loads one bounded page, cancels superseded reads and clears records after access denial. Explicit loads focus the announced count; failures focus the error. The initial automatic load preserves the inspection workflow's focus. Status is labeled as recorded deployment state, not live health or account authority. The existing bounded organization summary remains compatible and still controls empty-organization handover eligibility; an empty later page cannot make a populated organization eligible.

## Local qualification

Full smoke passed Node checks, frontend build and Go vet/race/PostgreSQL tests. Tests traverse 117 records over three pages with timestamp ties, verify fixed projection, cursor scope, revocation and former-owner denial, and reject CLI credentials. Optional golangci-lint remains unavailable.

Actual read-only drill `206a3a4b-2e0b-4aa5-a0e5-1621100e74e9` completed at 2026-10-01T17:34:35Z. Alice read four records, Bob read one and the empty collaboration organization returned none. Repeated reads and compatible-route projections matched; malformed/cross-identity cursors and CLI credentials were denied. Access audits, jobs, native repositories/media, containers and protected custody remained unchanged. Actual lists each contain one page.

The actual browser loaded Bob's retained native deployment. A temporary read-only browser response fixture exercised 50 synthetic records followed by one final record, keeping count focus after the pager disappeared. The fixture was removed, original fetch restored and real data reloaded; no source deployment was created. This qualifies browser behavior only. Five widths had no document/status overflow: 320×568, 375×812, 812×375, 768×1024 and 1440×900. Focused controls and errors had a visible outline when the preview document had focus.

At 17:40:03Z, disabling collaboration cleared records, focused the error and cleared busy state. Refresh hid collaboration while ordinary owner controls remained available. Private metadata remains in `.local/shared-instance-pages-browser.json`. T3 snapshots remain intermittent; assistive screen-reader and live qualification remain open. Source API runs on schema 24 with collaboration and other fixture permissions disabled; admission is open. No native container, volume or recovery copy was removed.
