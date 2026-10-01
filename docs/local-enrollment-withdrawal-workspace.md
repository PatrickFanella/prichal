# Browser invitation-withdrawal review

The first-account panel can review withdrawal of an issued, expired or failed invitation. The acknowledgement starts unchecked and must be selected before submission. The review explains the temporary restart and preservation of any account already created. Existing verified accounts do not show a withdrawal control. Closing review returns keyboard focus to its launcher.

Before submission, the tab stores only the retained enrollment UUID in session storage. Storage failure prevents submission. It immediately hides the invitation and account-creation link, disables replacement/status mutations and retains the same ticket if the response is unconfirmed. A fresh scoped status read is required before reviewing a retry. Reload never automatically submits or retains the acknowledgement.

An accepted management result distinguishes withdrawal recorded from native withdrawal confirmed. Pending confirmation blocks recovery; a confirmed withdrawn result offers separately authorized recovery. Failed withdrawal retains the native ambiguity and blocks replacement. At the three-request limit, the panel directs the owner to support rather than offering more retries. A completed account remains visible as account-created/verified status.

## Reads and stale results

Policy and status reads run in parallel with bounded abort signals. Mutation invalidates earlier reads so they cannot restore an old invitation afterward. Unmount/session changes abort mutation and ignore old completion. The latest status or retained exact operation ID must pass the [schema-28 contract](../web/src/enrollment-contract.ts), including identity, endpoint, code/state and withdrawal-attempt consistency.

Unavailable, invalid and scope-denied status clears stale controls and focuses an error. A retained unknown operation stays blocked until original-owner access or support resolves it. Polling only reads status; it does not issue, refresh or withdraw an invitation automatically. Preparing-request cancellation remains unavailable until a retained native invitation ticket is confirmed.

## Local browser evidence

The actual schema-28 workspace continued to show Bob's retained account as verified. Review, keyboard acknowledgement, lost-response retention, focused errors, retry reset, pending confirmation, confirmed withdrawal and retry-limit guidance passed in T3 browser fixtures. Two withdrawal POSTs were intercepted, and every non-read request was blocked from the source service. There were 150 intercepted fixture reads and zero source mutations.

Review fit 320×740, 375×812, 844×390, 768×1024 and 1440×900 CSS-pixel viewports without document/review overflow or controls outside the review panel. The checkbox remained unchecked through resizing. Snapshot still failed after reopening the attached preview; these are DOM/layout measurements, not visual or screen-reader qualification.

The original fetch function was restored at 2026-10-01T21:49:59Z. An actual reload at 21:50:25Z retained a nonexistent disposable ticket, focused its real 404 access error and rendered no withdrawal or account mutation controls. This checks unresolved-ticket reload without pretending the retained verified account has an unused invitation. The synthetic session-storage entry was removed; another actual reload at 21:50:30Z restored verified status with no fixture, review or withdrawal storage key.

Source deployment/job/enrollment/choice work and both native signed inventories/media/custody matched the baseline. No native account, invitation, domain, host resource or recovery copy changed. Private evidence is in ignored `.local/withdrawal-workspace-browser.json`. The full `./scripts/dev.sh smoke` gate and production web build passed. Contract regressions reject stale invitation codes, inconsistent withdrawal metadata, mismatched endpoints and malformed completed-account identity.

## Remaining gates

Actual browser withdrawal against a fresh empty PDS, native signup/drain/restart failure, preparing-request cancellation, visual review and assistive screen-reader checks remain open. The [backend workflow](local-enrollment-withdrawal-intent.md) and [restricted agent command](local-enrollment-withdrawal-agent.md) have their own evidence. This browser work does not prove live hosting, public signup or issue #26 completion. No occupied native slot was freed by deleting a retained tenant or recovery copy.
