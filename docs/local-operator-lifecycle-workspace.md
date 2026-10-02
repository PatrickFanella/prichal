# Local operator lifecycle recovery workspace

Status: implemented and locally verified on October 1, 2026. Production operator policy and live hosting remain unqualified.

The support workspace retains only an exact five-field lifecycle intent in owner-scoped session storage. Its instance, request key, target, desired state and reason stay locked until a confirmed terminal receipt is separately reviewed. Reload reads current operator permission and the exact audited receipt; it never submits automatically. Missing or malformed storage fails closed. A storage failure prevents submission.

A missing receipt allows only the original request after exact-instance inspection and a fresh unchecked acknowledgement. Pending receipts hide mutation and cleanup controls. Applied or failed receipts allow an unchecked browser-only cleanup review. Cleanup preserves the server operation, history and native state, clears inspected state and focuses confirmation. Inspection-only authority can read and close its own confirmed browser retry but cannot submit a lifecycle action.

Reads and mutations have eight-second deadlines and abort when session or scope changes. Invalid, unavailable or denied receipt responses discard stale controls. Inspection clears earlier evidence before fetching. A lost mutation response retains the original key and requires another receipt check and inspection before an exact retry. These controls complement server scope, idempotency, audit and recovery enforcement; browser state does not grant authority.

## Verification

The strict contract tests reject mismatched request, instance, target, desired state and reason, unsafe generation, unknown state, invalid dates and malformed or extra intent fields. Mutation acknowledgements are validated without inventing a returned request key. Production build and the full local smoke gate passed.

T3 keyboard and DOM checks verified absent-receipt review, lost-response retention, same-key retry, acknowledgement reset, pending lockout, applied/failed cleanup review, inspection-only mutation refusal, storage failure with no POST, invalid/unavailable receipt cleanup and confirmation/error focus. Two POSTs and thirty-six reads were intercepted; zero unexpected mutations occurred. No positive operator mutation or receipt lookup used the source API.

The review and controls fit the attached 320×740 preview without document overflow. Existing T3 resize timeouts leave four other widths and desktop restoration open. Snapshot failures also leave visual and screen-reader checks open.

Fixture fetch and storage overrides were removed at 00:19:58Z on October 2. Actual reload returned the default-closed operator workspace with no retained key. A malformed retained-key reload at 00:20:22Z preserved metadata and exposed no mutation controls; the synthetic key was then removed and the ordinary workspace reloaded. The earlier unknown exact-key reload also preserved its request without automatic submission under actual disabled operator policy.

Source management work, seven cancelled choices/fourteen audits, native signed records/media, container start times and custody matched the retained baseline. Source operator permissions stayed off; no source lifecycle operation, recovery hold, schema change, deletion or host mutation was introduced. Private evidence is in ignored `.local/operator-workspace-browser.json`, `.local/operator-workspace-source-after.json` and `.local/operator-workspace-smoke.log`.

Actual authorized operator qualification, production identity/MFA policy, the remaining browser checks and independent incident recovery remain open. See [server receipt enforcement](local-operator-lifecycle-receipts.md).
