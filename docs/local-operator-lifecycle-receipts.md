# Local operator lifecycle request receipts

Status: implemented and locally verified on October 1, 2026. [Browser recovery](local-operator-lifecycle-workspace.md) is locally verified with intercepted fixtures; production operator qualification remains open.

An authorized management-browser operator can GET `/api/operator/deployments/{instance}/lifecycle-requests/{key}` to inspect their own exact retained lifecycle intent. The fixed UUID request key and original `oidc-owner` actor bind the result. Another operator, tenant, request key or issuer cannot read it. Current inspection-only rights can read an original operator's own receipt, while lifecycle mutation still requires operate rights. Empty operator configuration keeps the route closed; local identity configuration cannot enable production policy.

The safe response contains operation and deployment UUIDs, the exact idempotency key, generation, kind, desired/prior state, reason, state and update time. It includes no native credentials, host metadata, actor identity, customer content or job data. A receipt is historical operation evidence, not new mutation authority. Each successful lookup commits an inspection audit before returning. Repeated reads add inspection audits but create no lifecycle intent or job.

Owner identity, deployment and original operation are locked for a bounded eight-second transaction. Current exact issuer/subject role is checked under the owner lock. Recovery is checked before lookup, after its lock and after auditing. Held/missing recovery, a revoked role or failed audit returns no receipt. An audit failure or recovery hold introduced during an audit wait rolls back the inspection too. The route accepts no query fields, refuses CLI bearer access and uses uncached generic error responses.

## Verification

The initial route regression returned 404 instead of the required exact receipt. Focused support race checks and full local smoke passed after implementation. Disposable fixtures verify exact repeated lookup and audit count, foreign operators/issuer/tenant/key refusal, malformed UUID/query and bearer refusal, disabled configuration, inspection-only original access, revoked roles, held/missing recovery, audit rollback and a hold introduced during audit-lock wait. Complete lifecycle operation/event/job rows remain unchanged throughout receipt reads. No mock or real native action was dispatched by lookup.

Positive reads used disposable browser-session fixtures. Ordinary source operator permissions remain off; production MFA/policy, actual authorized operator receipt/browser recovery and independent incident recovery remain unqualified. No source hold, policy enablement, tenant deletion or schema change was performed. Private evidence remains in ignored `.local/`.

The ordinary API restarted ready on schema 30. Its new receipt route returned the configured-off 404 response, with browser fixtures absent and source work/native/custody and saved-choice inventories unchanged.
