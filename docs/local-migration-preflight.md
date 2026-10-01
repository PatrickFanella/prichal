# Local outbound migration inventory

`GET /api/exports/{id}/migration-preflight` reads an existing owned, ready, unexpired portable archive. It requires the current organization owner and original export owner, a management browser session and local mode. Bearer authorization is denied. Responses use `no-store`. The API does not contact a source host, target host or identity directory.

The response reports archive version, capture time, archive bytes/hash, repository bytes, captured record count, blob count/bytes and preference coverage. AES-GCM scope authentication and every member hash must pass, and the retained management checkpoint must match. PostgreSQL stores capture times at microsecond precision; comparison truncates the authenticated manifest timestamp to that precision. Ownership, expiry, recovery status and checkpoint are rechecked after the read. Unavailable, expired, tampered, foreign or changed artifacts reveal no inventory.

`state` remains `archive_inventory_only`, with `canTransfer` and `canSwitchIdentity` false. The API reports `signatureVerification: not_rechecked`. Archive integrity is distinct from independently verifying the signed CAR and blob references, or proving current identity control. The record count is the captured manifest count. No archive contents, private preference values, signing keys, host credentials, storage paths or encryption keys appear in the response.

Version 1 reports `not_captured_v1`. Version 2 reports `captured_app_bsky_only`. This does not cover settings held by external applications or AppViews. Capture time is historical and does not prove that source data is current.

## Qualification

Start the separate disposable local PostgreSQL with `./scripts/dev.sh db`, then run `./scripts/dev.sh smoke`. Integration tests cover version 1/2 privacy, changed owner/expiry/checkpoint/recovery status during reading, cancelled request contexts, artifact tampering, missing custody, foreign access, bearer denial and production refusal. They compare retained export/event/job rows before and after inventory reads. Fake CAR test fixtures deliberately do not prove repository signatures.

With the existing local source services running, `node scripts/qualify-migration-preflight.mjs` reads the four retained exports. It creates no new export, resets no expiry or allowance and changes no native account. It compares the API inventory with the independent offline verifier in the existing pinned native container. The verifier receives archive bytes on stdin and reads no runtime credentials. This is operator qualification outside API execution; the API still has no Docker access.

Drill `48734021-122b-4944-9b22-f4247020b9e0` completed at 2026-10-01T20:02:34Z. Two version 1 and two version 2 archives passed. Alice had four captured records and one referenced blob; each Bob archive had six records and one blob. Exact repeat and foreign/bearer denial passed. Independent offline signature/media checks passed for these retained artifacts. Source work, encrypted artifacts and native inventory/custody matched the baseline. New local sign-ins intentionally created authentication activity. Private evidence remains in `.local/migration-preflight/`.

## Remaining migration gates

- Current source account authorization, supported recovery method and current signing identity.
- Independent repository signature, record and referenced blob verification for each transfer.
- Explicitly authorized target host, compatibility and capacity.
- Resumable import with independent inventory comparison and interruption/missing-blob drills.
- Explicit identity-switch authorization after verified transfer.
- Post-switch authentication, signing and downstream identity resolution.
- Preferences and application state held outside the exported PDS.

This completes a retained-archive inventory component of issue #29. It does not complete old-host access preflight, inbound/outbound migration or rollback qualification. No target or identity mutation is available. Schema remains 26 and no job kind or native resource is added. Existing recovery copies remain retained.
