# Owner hosting history

The owner workspace has one recorded timeline for deployment, first-account enrollment, domain proof, service/account actions, portable exports and hosting exit. It reads existing durable audit events and gives fixed next-step guidance. No new job, host call, email or impersonation action is created by reading history.

`GET /api/deployments/{id}/journey` requires the deployment's current organization owner. Operator authority alone does not grant this route, and CLI credentials cannot access it. Enrollment, export and exit events also require their original owner binding. Raw messages, domain values, actor identities, request keys, credentials and native private content are never selected. Unknown states become a generic review event.

Pages contain at most 50 events. The opaque `before` cursor preserves the full timestamp, category and integer event ID, including ties across tables. It binds the instance, accepts no arbitrary query or SQL and grants no authority. Each request rechecks current ownership within a read-only snapshot. History is not a frozen multi-page snapshot: newly recorded events appear on the next latest-page request. Historical export readiness and invitations do not promise current availability; guidance directs the owner to current controls and dates.

The browser loads history on request, labels timestamps and announces page counts. A new request cancels the previous request, and unmount/instance changes discard its result. It makes no mutation and does not resubmit an operation.

## Local qualification

Full smoke passed frontend build, Node checks and Go vet/race/PostgreSQL tests. Tests cover timestamp ties, all seven event categories, two-page traversal without loss or repetition, prior-owner revocation, operator non-bypass, private original-owner operations, cursor binding, malformed queries and safe fallback guidance.

Actual drill `22f25f60-15f3-4b06-bd62-f887d5c8068e` completed at 2026-10-01T15:06:40Z. Alice read 60 events and Bob read 69 events, each over two bounded pages. All seven categories appeared. Exact read retries matched. Foreign-owner access, cross-instance cursors, malformed query and CLI scope were denied. Audit counts, signed repositories/media, native container identities and protected credential/TLS/admission files remained unchanged. An initial verifier incorrectly treated the fixed instruction to save a private copy as a secret marker; the assertion now checks exact permitted fields and case-sensitive fixture markers. No source mutation was needed.

The later native T3 DOM check traversed Bob's 50-event latest page and 19-event final page with keyboard controls. The final page originally removed the focused pager and returned focus to the document body. Both hosting history and service/account activity now focus their announced page count after a successful load. The activity panel displayed 26 actions, and keyboard focus remained inside the loaded history workflow.

The expanded history panels had no document overflow or offscreen history elements at 320×568, 375×812, 812×375, 768×1024 and 1440×900. The owner page had no duplicate IDs or unlabeled visible inputs. Full smoke passed after the focus fix. Private DOM evidence remains in `.local/history-browser-widths.json`. Snapshot capture still fails on the existing preview client; visual appearance and assistive screen-reader behavior remain unqualified. Email notification, independent production storage, commercial billing and live support remain separate gates.

## Connected read feedback — October 28, 2026

On base `6850629`, the actual application reproduced an unclear refresh state: a pending history read disabled the load button without showing a loading message, and retained the previous four events. A controlled 503 then displayed an unfocused error without a retry instruction.

History reads now clear the displayed page and focus an explicit loading status. Failure focuses an alert with guidance to load the latest history again. Successful reads retain the existing focus on the page-count status. Clearing the previous page also removes its pager; focus moves to the loading status instead of falling back to the document body while an older-page request waits. Reading history remains a GET-only operation.

Playwright CLI qualification used the whole application, actual local OIDC sessions and API/worker binaries built from `6850629`, with a fresh disposable management database. Alice created an organization and submitted a deployment simulation; the real worker produced four persisted deployment events. History reads and successful retries used the real API. Only a history 503, a held response for timeout and delayed-response delivery were controlled at the browser transport boundary.

The following checks passed against the revised component:

- Keyboard-triggered loading showed focused status text, disabled the load button and removed the previous page. The controlled 503 focused retry guidance with a measured 3px outline.
- A keyboard retry read the same four persisted worker events from the API and focused their count. There were zero organization/deployment mutation requests during the read/failure/recovery checks.
- The unmodified eight-second read timeout produced focused error guidance, no old event page and an enabled retry. Removing the transport fault restored the real history.
- Error and recovered states had no document overflow or escaping history text/controls at 1440×900, 768×1024, 390×844, 320×568 and 844×390 CSS pixels.
- For pagination only, 52 explicitly synthetic audit rows were inserted into this disposable database alongside the four worker events. The real API returned pages of 50 and six. Keyboard navigation focused loading while the older-page response waited, then focused the final count after its pager disappeared. These rows provide no native provisioning or lifecycle qualification.
- A real HTTP 200 owner-history response was held until after Alice signed out. Delivering it late could not restore her history. Bob's subsequent real sign-in showed an empty workspace without Alice's organization or history.

Private evidence is `.local/issue-audit/connected-history/`, including baseline/results, controlled transport scripts, pagination seed receipt and source/binary hashes. The owned browser and issuer/API/web/worker processes were closed and the exact disposable management database was dropped. Retained native fixtures and data volumes were preserved. The separate smoke-suite database lifecycle and validation results are recorded in the progress log.

This adds connected local feedback and focus evidence for #24. It does not qualify screen-reader speech, physical devices, external identity providers, DNS/payment failures or the full native deployment/enrollment/export/exit journey. #24 and M4 remain open, including independent production serving and recovery.
