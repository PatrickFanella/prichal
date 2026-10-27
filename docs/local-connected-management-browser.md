# Connected local management browser qualification

## Scope — October 27, 2026

The actual Vite application was exercised through Playwright CLI against API, local OIDC issuer and River worker binaries built from `016d31951e44fbf2cda63d78d5ecbaaf11d3d2bf`, with a fresh disposable PostgreSQL database. The only application change during qualification was the server-name pattern described below. Authentication, organization creation, deployment persistence, authorization and worker transitions used the real source handlers and database. No native PDS or host agent ran.

The local issuer's Alice and Bob subjects share a fixture email but have separate identities. This qualifies the local OIDC/session path; it does not establish behavior of an external identity or mail provider.

## Observed journey

- Alice signed in through the local issuer and created one organization through the browser.
- With the worker stopped, a real deployment simulation POST committed one queued deployment and one River job. Its response was withheld by browser transport interception. A full page reload restored the pending name/mode and `Retry request` control. Manual retry returned the same deployment; database snapshots before and after retry retained one deployment, one queued event and one job.
- Starting the actual River worker advanced the operation through `queued`, `configuring`, `checking` and `ready`, with exactly one test instance. The browser displayed `Local test complete. No PDS has been deployed.` Full reload retained the operation.
- Alice signed out and Bob signed in. Bob's workspace was empty. A same-origin authenticated request for Alice's deployment list returned 404 without her deployment name or identifier. Signing back in as Alice restored the original organization and ready operation.
- The connected ready workspace had no document overflow at 1440×900, 768×1024, 390×844, 320×568 and 844×390 CSS pixels. These are desktop-browser viewport measurements, not physical-device qualification.

The initial interception helper expected HTTP 202 and raised after receiving the API's actual HTTP 200. It did not forward that committed response. Qualification continued by removing the interception, reloading and manually retrying the retained request. The database snapshots establish recovery without a duplicate intent or job; this was not a clean explicit-abort harness run. Other application responses were not synthesized.

## Server-name validation regression

The actual connected form reproduced a browser validation failure: pattern `[a-z][a-z0-9-]*` produced an invalid-pattern console error, and `Invalid_Name` incorrectly passed `checkValidity()` without a pattern mismatch. The pattern is now `[a-z](?:[a-z0-9]|-)*`, preserving lowercase initial letters followed by lowercase letters, digits or hyphens.

Browser checks against the revised form accepted `a`, `my-studio` and `studio7`; rejected `Invalid_Name`, `7studio`, `bad.name` and `bad name` with a pattern mismatch; and rejected an empty value as required. Pressing Enter with `Invalid_Name` kept focus on the invalid input and sent zero deployment POSTs. Server-side validation remains authoritative.

## Evidence, cleanup and remaining gates

Private evidence and helper logs are under `.local/issue-audit/connected-management/`: `before-worker.json`, `after-retry.json`, `worker-ready.json`, `final-database.json`, `browser-checks.json`, `isolation-checks.json`, `keyboard-checks.json` and the owned browser artifacts. The runtime registry contains private connection settings and is excluded from source control.

The owned browser and issuer/API/web/worker processes were closed. The exact disposable management database was dropped after all of its sessions exited. The separate verification database volume and all retained native tenant namespaces were preserved. Smoke-suite and hosted CI results are recorded separately in the progress log and PR.

This supplies connected local evidence for part of #24. Native provisioning, DNS/domain proof, payment/provider failures, enrollment, usage limits, export/cancellation and assistive-technology behavior across the full supported journey remain separate acceptance gates. It does not close #24, #25, #26 or M4. The homelab remains an interim private pilot; independent production serving and recovery still require the selected external environment and custody decisions.
