# First-account retry guidance

A retained first-account invitation request now has a persistent status explaining that retry sends the same request and never runs automatically. The retry button references this explanation through `aria-describedby`. The wording distinguishes an original invitation from invitation recovery using the retained request's recovery flag. Successful preparation moves keyboard focus to the returned operation status.

This changes guidance and focus only. The existing request key, storage, authorization, polling and server job contracts remain in place. A confirmed preparation response clears the retained request as before; pending native preparation still needs its later status confirmation.

## Controlled browser evidence — October 20, 2026

On base `bb90302`, the actual `FirstAccount` component reproduced two gaps: remounting after a lost response restored the retry request without its explanation, and a confirmed retry left focus on the page body when the submit button disappeared.

The revised component passed these checks in the T3 browser with synthetic loopback API responses:

- Lost-response guidance and the linked retry description survived a component remount. Remounting issued no POST. Repeated manual retries kept the original request key and recovery flag.
- A confirmed preparation response cleared browser request storage and removed the retry guidance. Its pending operation status received focus.
- A retained recovery request used the recovery wording and kept its exact key and `recover: true` payload through pointer and keyboard retries. Failures focused the existing alert.
- Full same-origin navigation, followed by mounting the controlled component, restored the retained recovery request and linked description with zero POSTs.
- The retry error state had no document overflow at CSS viewports 320×568, 568×320, 375×812, 768×1024 and 1440×900. The description resolved to its status element, and the focused alert kept its existing 3px outline at every width.

Private evidence is `.local/issue-audit/enrollment-retry-browser.json`, including qualified source hashes. These checks use the real React component and styles, but do not qualify real invitation delivery, native account creation, physical devices, screen-reader speech or the complete onboarding journey. No account, tenant or production service changed. Issue #24 remains open for its wider acceptance criteria and dependencies.
