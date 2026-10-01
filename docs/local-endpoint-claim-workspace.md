# Local endpoint claim workspace

Status: implemented and locally verified on October 1, 2026. Public reservation and hosting activation remain unqualified.

The owner workspace reads the exact saved choice and its claim receipt before exposing an unchecked reservation review. Confirmation sends an empty request for that choice UUID. Session storage retains only the choice UUID and hostname before submission, scoped by owner and organization. Reload performs reads and never automatically submits a claim. Uncertain results pause other address changes until a fresh scoped read permits a separate unchecked retry.

The retained claim panel remains available if another tab cancels the current choice. Cancelled, expired and local test choices cannot open a new reservation review. A separate unchecked review can close only the browser retry after current scope and state are known. Malformed browser metadata blocks changes and requires this same explicit removal review. Server choices and receipts are preserved. Reserved, released and expired receipts never authorize hosting activation.

Unavailable, invalid and denied reads remove stale receipt and review controls. Proof expiry during review removes confirmation authority and focuses the error. Storage failure prevents submission. Read and mutation responses are guarded against aborted or changed scope. Confirmation and browser-only cleanup focus their result; closing a review restores its launcher.

## Verification

The full `./scripts/dev.sh smoke` gate and two claim-contract tests passed. T3 browser keyboard checks covered unchecked review, lost response, fresh-read retry, other-tab cancellation, browser-only cleanup, confirmed reservation, released/expired receipts, invalid activation authority, proof expiry, storage failure, unavailable reads and denied scope. Positive reservation responses were intercepted browser fixtures, not public DNS or source reservations. The fixture intercepted three POSTs and 39 reads, blocked zero unexpected mutations, and was removed before real reload checks.

Real reload checks retained an unknown exact choice while denying mutation controls. Malformed metadata opened the recovery panel and required unchecked review; closing it removed the key and focused confirmation. An initial focus failure on this path was fixed and the keyboard check repeated successfully. Native fetch was restored, synthetic keys removed and the ordinary workspace reloaded.

Review panels fit at 320 x 740, 375 x 812, 844 x 390, 768 x 1024 and 1440 x 900 without document/panel overflow or controls outside the panel. T3 snapshots failed, so visual and screen-reader qualification remain open. No browser fallback was used. Source before/after inventories matched seven cancelled choices, fourteen choice audits, zero pending choices and the preserved work/native/custody hashes. No native tenant, occupied slot, export or recovery copy was removed or reset. Safe private evidence remains in ignored `.local/`.

## Remaining gates

Controlled public DNS, positive populated-claim recovery, route/TLS provisioning, ownership revalidation and hosting activation remain open. Claims are local-only and every receipt retains `canActivate=false`. This workspace does not change native account identities or implement account migration.
