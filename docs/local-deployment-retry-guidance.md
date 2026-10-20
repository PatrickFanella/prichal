# Deployment retry guidance

The server form keeps deployment submission errors separate from background status-read errors. A successful poll clears only a status-read error; it cannot clear a failed submission's explanation. Background failures continue to clear the displayed deployment list as before, so this change does not retain unverified deployment controls.

An uncertain request has persistent, plain-language guidance explaining why its name and deployment mode are kept. The retry button references that guidance through `aria-describedby`. This explanation also appears when the component restores a pending request from browser storage. Submission errors receive focus and a visible outline; background polls do not move focus.

Lost responses, invalid success receipts and server errors retain the request for retry. Server errors now also set the in-memory uncertain state, keeping name and mode locked to the saved payload. A definitive 4xx rejection keeps the existing behavior of clearing the pending request and allowing a new one. A validated success clears the retained request and explanation. Polling never submits a request.

## Controlled browser evidence — October 20, 2026

On base `3befc24`, a real T3 browser reproduced the defect in the actual `Deployments` component: a lost POST response showed the retry error immediately, then the next successful three-second poll removed it while the retry button and stored request remained. The private before-result is `.local/issue-audit/deployment-retry-before.json`.

The revised component passed the following controlled checks with its actual React code and styles:

- Lost-response guidance survived successful polling, a failed status poll and subsequent recovery. The status-read failure appeared alongside the submission error, then cleared independently. No automatic POST occurred. Three manual lost-response submissions retained identical payloads and request keys.
- A full same-origin browser navigation, followed by remounting the controlled component, restored the stored name/key/mode and linked retry explanation with zero POSTs.
- A keyboard-activated 400 rejection cleared pending storage, removed the linked description and unlocked name/mode. The error received focus.
- A 503 retained the pending payload, locked name/mode and preserved its error and explanation through polling. A repeated keyboard retry retained the same payload and focused the error with a 3px outline.
- A later confirmed response used the same payload as the preceding server-error attempts, showed the ready result, removed pending storage and its description, and unlocked the form. Polling added no POSTs.
- The error and retry state had no measured document overflow or escaping component elements at 320×568, 568×320, 375×812, 768×1024 and 1440×900. Visible input/select controls had labels, the retry description resolved to its status element, and focused errors retained the outline at every width. The accessibility snapshot exposed the retained explanation as a status and the locked controls as disabled.

Private evidence is `.local/issue-audit/deployment-retry-browser.json`, including exact source hashes. These are desktop-browser CSS viewports and synthetic API responses in the actual component. They do not qualify physical devices, screen-reader speech, real provisioning or the full DNS/payment/account journey. No management service, tenant or production route was mutated. Issue #24 remains open for its wider acceptance criteria and dependencies.
