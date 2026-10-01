# Owner hosting history

The owner workspace has one recorded timeline for deployment, first-account enrollment, domain proof, service/account actions, portable exports and hosting exit. It reads existing durable audit events and gives fixed next-step guidance. No new job, host call, email or impersonation action is created by reading history.

`GET /api/deployments/{id}/journey` requires the deployment's current organization owner. Operator authority alone does not grant this route, and CLI credentials cannot access it. Enrollment, export and exit events also require their original owner binding. Raw messages, domain values, actor identities, request keys, credentials and native private content are never selected. Unknown states become a generic review event.

Pages contain at most 50 events. The opaque `before` cursor preserves the full timestamp, category and integer event ID, including ties across tables. It binds the instance, accepts no arbitrary query or SQL and grants no authority. Each request rechecks current ownership within a read-only snapshot. History is not a frozen multi-page snapshot: newly recorded events appear on the next latest-page request. Historical export readiness and invitations do not promise current availability; guidance directs the owner to current controls and dates.

The browser loads history on request, labels timestamps and announces page counts. A new request cancels the previous request, and unmount/instance changes discard its result. It makes no mutation and does not resubmit an operation.

## Local qualification

Full smoke passed frontend build, Node checks and Go vet/race/PostgreSQL tests. Tests cover timestamp ties, all seven event categories, two-page traversal without loss or repetition, prior-owner revocation, operator non-bypass, private original-owner operations, cursor binding, malformed queries and safe fallback guidance.

Actual drill `22f25f60-15f3-4b06-bd62-f887d5c8068e` completed at 2026-10-01T15:06:40Z. Alice read 60 events and Bob read 69 events, each over two bounded pages. All seven categories appeared. Exact read retries matched. Foreign-owner access, cross-instance cursors, malformed query and CLI scope were denied. Audit counts, signed repositories/media, native container identities and protected credential/TLS/admission files remained unchanged. An initial verifier incorrectly treated the fixed instruction to save a private copy as a secret marker; the assertion now checks exact permitted fields and case-sensitive fixture markers. No source mutation was needed.

Collaborative preview status reports the local page available at 375 by 812, but snapshot still fails on client `preview-2159aa7e21ca3525611142bfa246cff8`. Responsive rendering, keyboard/screen-reader behavior and reopened browser state remain unqualified. Email notification, independent production storage, commercial billing and live support remain separate gates.
