# Checking saved TXT instructions from the browser

A pending choice now includes a separate read-only TXT observation control. The request uses only its current organization/choice reference and browser CSRF; no resolver or destination is supplied. Typed results bind the exact choice/hostname, require consistent match state and false claim/activation flags, and display the database check time. Positive fixture results still grant no exclusive claim or activation.

Reads are bounded and discarded after unmount, choice change or action review. Review disables the DNS control and immediately hides its previous result/error while the controller aborts and clears state. Failure clears the result and focuses a fixed error. Authorization denial clears the parent choice and acknowledgement, removes cached TXT instructions and focuses the access error. Refresh must confirm current owner state before proceeding.

Phone organization rows put the icon/badge above the full-width content. Address paragraphs use readable text size, and TXT name/value fields stack at every width. The desktop status card's grid remains separate.

## Verification

Full smoke passed the initial control, immediate-result-hiding change and final phone/grid style change. Actual T3 keyboard observation fixture at 19:34:18Z focused the result and denied claim/activation authority. Unavailable response at 19:35:08Z cleared the result and focused error; entering cancellation review disabled the read and cleared its result. These were scoped read-only response fixtures with zero source mutation attempts.

T3 repeatedly lost its automation host during resize. After `preview_open` explicitly reported no host, an isolated headless Chrome context completed the remaining checks. The Blacktower Playwright workflow uses the existing Chrome installation, a fresh ignored local profile and pinned Playwright 1.63.0. No browser download or application toolchain change was needed. This follows the [Playwright library workflow](https://playwright.dev/docs/library).

The first headless check exposed asynchronous result clearing during review. Rendering now hides it immediately whenever review disables the control. An initial passing screenshot then showed cramped phone content despite no overflow. Moving content below the icon/badge and stacking TXT fields fixed that layout. Final drill `465060fb-8c65-49a5-ba59-b3617fcb8d6c` completed at 19:45:15Z on Chrome 154.0.8037.92.

The final drill passed keyboard result focus, unavailable-result/error behavior, immediate review clearing, exact retained review across reload with acknowledgement unchecked, DNS access denial and saved-choice read denial. At 320×568, 375×812, 812×375, 768×1024 and 1440×900 there was no document/panel/choice overflow or out-of-panel control. Panel widths were 230 and 285 pixels on the two phones, compared with the prior cramped column. The 375-pixel screenshot was inspected. Four DNS response fixtures ran, zero source mutation requests were attempted, all routes were removed, actual null choice reloaded and the isolated profile closed. Private evidence remains under `.local/browser-qualification/runs/465060fb-8c65-49a5-ba59-b3617fcb8d6c/`.

T3 reconnected later. Its original fetch was restored and actual null state loaded before the retained synthetic cancellation was cleared at 19:46:38Z. Shared-browser fixture cleanup is now complete. The earlier 19:33:19Z cleanup is also retained as evidence; a readiness repeat corrected an initial check that ran before the refreshed state rendered. Metadata remains in `.local/endpoint-observation-controls-browser.json`.

Source checks after the browser work preserved six cancelled choices/twelve audits, zero pending choices, jobs/endpoints/admission policy and native repositories/media, containers and protected custody. No native container, volume or recovery copy was removed. The schema remains 26; no new job kind was added. Positive public DNS, exclusive verified claims, routing/TLS/activation, assistive screen-reader behavior and account migration remain unqualified.

## Running the isolated fallback check

Use it only when the native T3 preview is unavailable. Chrome must be installed, the local issuer/API/Vite must be running and the fixed disposable owner/organization fixture must still exist. This script blocks source API mutations and external origins; it does not operate live infrastructure.

```sh
mkdir -p .local/browser-qualification
printf '%s\n' '{"private":true,"type":"module","name":"pdsaas-local-browser-qualification"}' > .local/browser-qualification/package.json
pnpm --dir .local/browser-qualification add --save-exact playwright@1.63.0
node scripts/qualify-endpoint-browser.mjs
```

The script creates a fresh local profile/report for each run, captures a phone screenshot, closes its context and retains artifacts. Authentication stays under the private ignored run directory. Do not use a personal browser profile or redirect the fixed local targets to production.
