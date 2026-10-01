# Reviewing custom-address choices in the browser

The organization address panel now reads the current pending choice and offers separate review for saving a successful empty-organization plan or cancelling its saved choice. Existing native preflight never offers save. DNS verification and activation remain unavailable.

The browser persists each exact request in owner/organization-scoped session storage before review and submission. Reload opens its review panel, keeps confirmation unchecked and sends no mutation. Save/cancel retries retain their key and canonical payload. A failed or uncertain response clears cached current state, keeps the request and requires a fresh state read and acknowledgement before retry. A successful typed receipt clears the browser intent; cancelled save replay cannot resurrect a choice. Failure to store or validate retained intent prevents a new request.

Reads have bounded abortable requests and discard superseded/unmounted responses. Refresh clears cached choice, notice and acknowledgement before reading. Owner identity changes remount the panel with a distinct storage scope. Server ownership/Origin/CSRF still determine authority. TXT instructions display only fixed typed fields and never imply ownership proof or activation. Clearing a browser request preserves any server-side choice; cancelling requires a separate reviewed mutation.

## Actual local checks

Full smoke passed the initial implementation and the final notice-clearing change. Actual preflight/staging focused review at 19:05:19Z. Reload at 19:05:44Z reopened that panel, left acknowledgement unchecked and sent no mutation. Keyboard submission after checking readiness saved the choice at 19:06:58Z, focused its receipt and showed scoped TXT instructions plus the hosting/handover pause. Reload read the saved state without a mutation.

Cancellation review focused its heading at 19:08:46Z and began unchecked/disabled. Keyboard confirmation completed at 19:09:14Z, removed current choice/review, cleared retained intent and focused the receipt. One new cancelled choice/two audits were added, leaving five choices/ten audits and zero pending choices. Management work, deployment endpoints, native repositories/media, containers and protected custody matched the baseline.

A temporary unavailable-response fixture at 19:10:32Z intercepted one save attempt and sent zero source mutations. It retained the exact request, cleared current state, focused the error and reset acknowledgement. Original fetch was restored. Reload at 19:11:02Z retained the same hashed intent and sent no mutation. After an actual state read and keyboard acknowledgement, clearing the browser request passed at 19:12:18Z. Some preview mouse attempts failed or did not change acknowledgement; readiness and keyboard repeats established the completed behavior. DOM opening was used for cancellation after collapsed-panel click attempts failed.

A separate read-only fixture displayed a 244-character hostname, TXT instructions and an unchecked synthetic cancellation review. At 320×568 there was no document/panel/choice overflow or out-of-panel control. The T3 browser connection disappeared before the other four widths and cache-denial check. Those checks remain open. Snapshot capture also failed; no new image or assistive screen-reader qualification is claimed.

The disconnect prevented restoration of that last browser fixture. It intercepts scoped endpoint mutations rather than forwarding them; the synthetic cancellation intent cannot submit automatically. Source checks after the disconnect still showed five cancelled choices/ten audits, zero pending choices and unchanged management/native work. The current browser may retain the fixture and synthetic intent; do not treat its displayed address as actual server state.

## Subsequent qualification

The earlier disconnect is historical. The remaining five-width and cache-denial checks passed in an isolated headless browser after T3 explicitly reported no automation host. The shared browser reconnected, original fetch was restored, actual null choice loaded and its synthetic request cleared at 19:46:38Z. See [observation controls](local-endpoint-observation-controls.md) for exact evidence, the mobile layout fix and remaining gates. Shared-browser fixture cleanup is complete.

## Browser cleanup after reconnect

Restore the original fetch saved as `window.endpointWidthOriginalFetch`, if it exists, or navigate afresh to remove the temporary function. Refresh saved choice and wait for the actual null result. The retained cancellation is for synthetic choice `55555555-5555-4555-8555-555555555555` under local organization `a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c`. Review and clear that browser request through the UI after the actual read. Do not submit its cancellation. Then rerun the remaining viewport and denied-read checks. Private metadata remains in `.local/endpoint-workspace-browser.json`.

Schema remains 26. The earlier 49-table recovery checkpoint includes four API-drill receipts/eight audits; the later browser receipt follows that snapshot. Local services retain fixture permissions disabled and both admission policies open. No native container, volume or recovery copy was removed. Ownership verification, verified-claim conflicts, routing/TLS/activation and migration remain open.
