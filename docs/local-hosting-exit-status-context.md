# Hosting-exit status and request context

Hosting-exit controls require a successful current policy, exit-history and export read. A failed read clears the displayed policy/history/exports, pauses acknowledgement, scheduling, reversal, retry and retained-request reset, and explains that actions are waiting for current status. Successful polling restores access without clearing a submission error or reusing acknowledgements.

A changed policy timeline also clears acknowledgements. New scheduling requires a currently available export and no conflicting exit. Retained retries keep their original key, export and timeline. Clearing a retained request requires fresh status and review; a browser storage failure reports an error rather than discarding the request.

Reads started before a submission cannot replace its result. Polling waits during submission, and a synchronous guard prevents overlapping actions. Deployment/session changes abort pending actions, clear displayed state and restore the new context's retained request. A late response cannot populate the new context or remove the old context's request. Action errors receive visible focus; background reads do not move focus.

## Controlled qualification — October 26, 2026

On base `dc180b1`, the actual component reproduced an unsafe stale-control state: after a successful policy read and both acknowledgements, a failed poll displayed an unavailable error while `Schedule hosting exit` remained enabled with the previous timeline. The baseline sent zero POSTs.

Playwright CLI checks against the revised actual React component and styles passed:

- Read failure removed scheduling and disabled the remaining acknowledgements. Recovery required both acknowledgements again. A later timeline change reset them again.
- A lost submission response retained the exact request. A following read failure disabled retry and reset without changing that request. Read recovery preserved the submission error. A confirmed retry used the same payload, displayed the scheduled result and cleared its browser metadata.
- An older deferred history response could not replace a newly confirmed reversal result.
- Switching deployment during a pending submission aborted it. Its eventual response did not populate the new deployment or remove the original deployment's retained request. Returning to the original context restored that request with unchecked acknowledgements.
- Full same-origin page reload and controlled remount preserved the exact request key and original timeline with zero POSTs and unchecked acknowledgements.
- The retained-request state had no measured document overflow or escaping elements at 320×568, 568×320, 375×812, 768×1024 and 1440×900 CSS pixels. An action error received focus with the scoped outline.

Private evidence is `.local/issue-audit/exit-status-browser.json`, including before/after results and qualified source hashes. The API responses and recorded actions were synthetic loopback fixtures; no actual hosting exit, reversal, payment cancellation or data deletion occurred. These checks do not qualify real hosting lifecycle behavior, screen-reader speech, commercial policy or the complete connected onboarding/exit journey. Issues #21 and #24 remain open.

The first post-edit run encountered an intermediate cached Vite transform. The final checks used a fresh page load after confirming the served transform contained the final controls. The full pinned smoke gate passed separately. The owned browser, temporary dev server and verification database were stopped after qualification; retained volumes and unrelated services were preserved.
