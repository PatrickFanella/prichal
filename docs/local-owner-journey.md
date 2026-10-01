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
