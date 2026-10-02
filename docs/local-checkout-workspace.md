# Local checkout review workspace

Status: implemented and locally verified on October 1, 2026. This is an opt-in fixture workflow, without commercial hosting entitlement.

Each owner's organization has a checkout review panel. An authenticated read-only catalog names the configured fixture Price and whether preparation is available. The read locks current ownership, checks recovery and reports unavailable preparation when checkout, billing or pending handover work already exists. It queues nothing and makes no provider call.

The browser retains exactly one original key and fixture Price under an owner/organization-scoped session-storage key. Preparation and dispatch have separate unchecked reviews. Preparing never dispatches; every mutation response requires a fresh receipt read before another step. Lost responses keep the same original request. Refresh reads catalog and original receipt together; a missing receipt permits only the same preparation when the catalog still allows it. A changed Price, invalid/denied/unavailable response or malformed metadata hides mutation and navigation. No POST is automatically replayed.

Queued/customer-ready work offers receipt refresh only. Confirmed session recovery reads the existing session. Open links remain transient, use no-referrer and disappear at expiry; no URL is written to session storage. Closed/invalid sessions expose no link and grant no entitlement. Manual-review work cannot renew or replace a request. A separate unchecked terminal cleanup removes browser metadata only, preserving the server receipt, jobs and remote resources. Invalid metadata remains retained for support review.

Full smoke, production build, focused HTTP races and two strict contract tests passed. Catalog HTTP checks verified current ownership, recovery, stable read-only availability and refusal to prepare over an existing request. Contracts reject foreign scope, changed catalog, payment claims, unknown fields and unsafe/closed/expired navigation.

T3 browser fixtures exercised three explicit intercepted POSTs and fourteen GETs with zero unexpected calls: lost preparation and original-key retry, a fresh separate dispatch review, lost queue response, pending lockout, transient open navigation, closed/invalid navigation suppression and reviewed browser-only cleanup with error/status focus. A separate storage-failure fixture sent zero POSTs. These browser responses were intercepted fixtures, not source dispatch or an actual billing authority journey.

Actual reloads with valid unknown and malformed test metadata returned disabled catalog 503, retained metadata and exposed only receipt refresh, without navigation or submission. Test-owned metadata and fetch/storage overrides were removed, then the actual workspace reloaded. Source request/audit tables remain empty, jobs remain seventy-nine, API readiness remains green on schema 32 and source work/custody matches. Ordinary billing sessions remain disabled.

Measured 320-pixel phone and 768-pixel tablet layouts fit without page overflow. The preview resize connection timed out while changing to 375 pixels; 375, landscape phone, desktop and desktop restoration remain open. Reopening restored inspection and allowed fixture cleanup. The tab remains 768 by 1024. Optional golangci-lint remains unavailable.

Private evidence remains in ignored `.local/checkout-workspace-build.log`, `.local/checkout-workspace-http.log`, `.local/checkout-workspace-smoke.log`, `.local/checkout-workspace-browser.json`, `.local/checkout-workspace-source-after.json` and `.local/checkout-workspace-source-counts.json`. See [owner routes](local-checkout-owner-routes.md), [dispatch](local-checkout-dispatch.md) and [session recovery](local-checkout-session-recovery.md).

An actual isolated local authority journey, populated checkout restore, signed completion mapping into authoritative reconciliation, commercial entitlement/grace policy, actual Stripe sandbox and live qualification remain open.

On October 1 a fresh shared-browser tab resolved the resize connection failure. Actual disabled checkout panels fit 375 portrait, 667 landscape and 1280 desktop widths with no document/control overflow, no checkout link and no retained test metadata. The current preview returned to 1280 by 800. This follow-up covers the disabled state; positive fixture states at these additional widths and an interactive actual-authority journey remain open. Private follow-up evidence is in ignored `.local/checkout-responsive-followup.json`.
