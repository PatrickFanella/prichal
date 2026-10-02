# Local activity recovery and scope

Status: implemented and locally verified on October 1, 2026. Production operator and independent recovery gates remain open.

Activity history now locks the deployment and organization scope for one bounded read transaction. Current owner access is held through page materialization. Recovery is checked before and after the page query using the current database state. A held or missing ledger returns no page. The transaction and request have an eight-second deadline; no activity, inspection or lifecycle audit is written by the owner history read. Existing operator inspection auditing and exact issuer/subject authorization remain in place.

The activity route accepts no cursor or exactly one nonempty positive `before` integer within JavaScript's safe integer range. Duplicate, unknown, malformed, empty and unsafe cursors are refused before history or operator inspection is read. Store callers are also bounded to the safe cursor range. Cursor and recovery refusal responses remain generic.

## Verification

Held/missing-ledger reads and ambiguous cursor requests first reproduced the old disclosure and acceptance behavior. Focused support race checks and the full local smoke gate passed after the fix. A page query blocked on a table lock held owner custody against concurrent transfer; after the read completed, transfer succeeded and the former owner was denied. A recovery hold inserted during the page wait caused the final read to return no data. These tests used only allocator-owned disposable databases. An initial test-helper name compile error was corrected before the behavioral regression run.

No schema, host credentials, native operation or lifecycle authority was added. Existing actual source containers, signed records/media, custody and saved-choice counts remain preserved. Private verification evidence remains in ignored `.local/`. Public operator configuration, live identity custody, assistive browser checks and independent incident recovery remain unqualified.

The updated ordinary local API returned Bob's twenty-six retained actions, refused duplicate/unknown/empty/unsafe cursor queries with 400 and denied the foreign tenant and unconfigured operator routes with 404. Source work/native/custody and saved-choice inventories matched before and after these read-only checks. No browser fixtures remained.
