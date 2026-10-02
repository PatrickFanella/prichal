# Local lifecycle history workspace

Status: locally implemented and verified on October 1, 2026. Four additional viewport checks and visual/screen-reader qualification remain open.

Owner and operator history now use a separate bounded read-only panel. Each refresh clears the old rows and cursor before requesting the exact deployment scope. Failed, denied and malformed responses leave no old history or pagination controls. Scope changes abort pending reads and reset state; stale responses cannot publish into the new scope. An eight-second deadline bounds each read, and a request guard prevents duplicate loads. Results and failures receive keyboard focus.

The response contract requires up to fifty unique UUIDs, strictly descending safe integer generations, supported service/account actions, known states/reasons and a cursor equal to the final visible generation only when a full page has more results. Older pages must precede the requested cursor. Owner results require redacted `operator` actor values; operator projections accept bounded actor labels. No mutation controls or hosting authority are added.

## Verification

Contract tests, the web build and full local smoke gate passed. T3 keyboard checks loaded the actual twenty-six retained Bob service/account actions and focused the result. Intercepted read-only fixtures verified unavailable refresh removing stale rows, fifty-to-two pagination, final-page cursor removal, invalid-cursor rejection and denied-scope cleanup, with errors focused. Six fixture reads and zero unexpected mutations were recorded across two fixture segments. Native fetch was restored, the fixture removed and actual twenty-six-row history reloaded. Source work/native/custody and saved-choice inventories matched before and after.

The fifty-row page fit at 320 x 740 without document or panel overflow or controls outside the panel. T3 automation lost its host during resizing; reopening recovered reads and keyboard interaction, but subsequent resize requests timed out, including restoration to desktop size. The remaining four representative widths are unqualified. No browser fallback was used. Visual/screen-reader checks remain open. Private safe evidence remains in ignored `.local/`.
