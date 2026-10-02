# Local owner checkout routes

Status: implemented and locally verified on October 1, 2026. Browser review and actual fixture authority journey remain open.

The API separately loads `PDS_BILLING_SESSIONS_MODE`. Empty/disabled mode exposes no checkout action to an authenticated owner and returns 503 without loading credentials or contacting billing authority. Enabling the existing signed notification inbox alone does not enable these routes. Fixture mode still requires explicit local authentication, protected fixture credentials and fixed numeric-loopback catalog/return configuration. No production or external billing fallback exists.

All routes require a management browser session. Authorization headers, including empty headers, are rejected before session authorization. Preparation and start require the existing exact Origin/CSRF check. Current/original organization ownership and recovery remain enforced by the durable store. Responses use no-store, nosniff and no-referrer headers.

| Route under `/api/organizations/{id}/billing/checkout` | Behavior |
| --- | --- |
| `POST` | Review and prepare the configured fixture plan with exactly `idempotencyKey` and `confirmed: true`. No remote call or job. |
| `GET ?key={originalKey}` | Recover the exact original receipt after a lost preparation response. |
| `GET /{checkout}` | Read the original owner's safe receipt. |
| `POST /{checkout}/start` | Require exactly `confirmed: true`, then atomically queue the original request/audit once. No synchronous remote call. |
| `GET /{checkout}/session` | Read the confirmed original session, returning transient navigation only when verified open. No replacement session or entitlement. |

Body parsing rejects duplicate/unknown/null fields, absent or false confirmation, trailing JSON, invalid keys and bodies over 2 KiB. Query parsing rejects duplicate/unknown keys and malformed encoding. Read routes reject bodies. Browser input cannot choose a customer, session, Price, return destination or credential. Portal sessions and commercial cancellation are not exposed.

Focused race tests used the real management authorization service and disposable PostgreSQL sessions. They verified cookie authentication, Origin/CSRF, bearer refusal, strict parsing, stable preparation/read replay, explicit one-job queueing, no synchronous authority calls, foreign custody, recovery holds, disabled routes and transient session recovery. Complete checkout/job/audit ledgers stayed unchanged after rejected and read-only requests. Focused function coverage was 91.2% for body parsing and 100% for response/error/route handling.

The full local smoke gate passed. The API was restarted with ordinary session billing disabled and remains ready on schema 32. Actual authenticated disabled-route checks returned 503, incorrect-CSRF mutation returned 403 and bearer access returned 403 without creating work. Source checkout request/audit tables remain empty; work/custody and seven cancelled choices/fourteen audits match the baseline. No native action or actual Stripe resource occurred. Optional golangci-lint remains unavailable.

Private evidence remains in ignored `.local/checkout-http-focused.log`, `.local/checkout-http-cover.out`, `.local/checkout-http-smoke.log`, `.local/checkout-http-source-after.json` and disabled runtime checks. This adds no schema or live configuration. See [dispatch](local-checkout-dispatch.md) and [read-only session recovery](local-checkout-session-recovery.md).

Browser review, an actual isolated local fixture journey, populated checkout restore, signed completion mapping into authoritative reconciliation, commercial entitlement/grace policy, actual Stripe sandbox and live qualification remain open.
