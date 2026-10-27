# Owner workspace read context

Workspace refreshes cancel their predecessor and accept results only from the current read. The component checks that identity after session and organization responses, including response-body decoding. Older successes, failures and 401 responses cannot replace newer workspace state.

Organization creation and sign-out cancel outstanding workspace reads before mutation. Confirmed sign-out, including an already-expired session response, also cancels any read started while sign-out was pending. Unmounting cancels the current read. These guards affect displayed state; server authorization and session revocation remain authoritative.

An unavailable workspace now asks the customer to refresh current state before creating another organization. It no longer claims that nothing changed: a creation can succeed before the following workspace read fails. This change does not add automatic creation retries or alter the organization API's request contract.

## Controlled qualification — October 27, 2026

On base `4f7ed0a`, the actual `OwnerWorkspace` component reproduced two failures. A deferred organization refresh returned after confirmed sign-out and restored the signed-in screen. Separately, a confirmed creation followed by a failed read displayed `Nothing was changed` despite the fixture retaining the new organization.

Playwright CLI checks against the revised actual component passed:

- Both `Sign out` and `Sign out all devices` aborted the pending read. A transport deliberately returning after cancellation could not restore the signed-in heading or creation controls.
- A newer successful refresh remained visible after an older deferred session response returned 401.
- A confirmed creation followed by read failure displayed recovery guidance without a no-change claim. A manual refresh recovered the same organization; only one creation POST occurred.
- The unavailable state had no document overflow or escaping elements at 320×568, 568×320, 375×812, 768×1024 and 1440×900 CSS pixels.

Private evidence is `.local/issue-audit/owner-context-browser.json`, including baseline results, after-fix assertions and qualified component/style hashes. Replies and sessions were synthetic loopback fixtures. These checks do not qualify external identity-provider revocation, real account sessions, screen-reader speech or the complete connected onboarding journey. No real organization or session was changed; #24 remains open.

The initial full smoke run failed during cleanup of a disposable endpoint-plan database with SQLSTATE 55006; its test body reported no assertion failure. Inspection found admission fenced and no remaining sessions. Only that named fixture database was removed, and the full endpoint-plan package passed independently. The complete pinned smoke rerun then passed with `GOFLAGS=-p=2`: all 55 Node checks, frontend build, Go vet and all race-enabled Go packages remained enabled. No backend cleanup or CI configuration changed. Private logs retain both runs and the focused rerun; hosted CI remains separate.
