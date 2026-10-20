# Domain instruction request context

Domain setup cancels its initial status read when the customer edits or submits the domain. An aborted response cannot replace newer input or instructions, even when a transport returns after cancellation. Changing the deployment or session context also cancels outstanding reads and submissions and clears their displayed state.

The form disables editing during a submission. Editing a saved domain hides its TXT record and account-check control until the edited domain is saved, with an explicit unsaved explanation. This keeps instructions tied to the domain currently shown. Initial status failures have a separate retry control. Reads and submissions have an eight-second timeout; action failures and successful results receive visible keyboard focus. Background reads do not move focus.

## Controlled browser evidence — October 20, 2026

On base `93d298c`, the actual `HandleSetup` component reproduced an out-of-order response bug: a save for `new.example.org` completed, then a delayed initial read replaced its input and TXT record with `old.example.org`. The private baseline is `.local/issue-audit/handle-context-before.json`.

The revised component passed these checks in the T3 browser with its real React code and styles and synthetic API responses:

- A delayed initial read was aborted and ignored after a keyboard-submitted save. Exactly one POST was made; the saved input and TXT record remained unchanged after the late read. The input was disabled while that save was pending and the result received focus.
- Editing the saved domain removed the old TXT instructions and DNS-check control and displayed the unsaved explanation.
- Switching deployment context during a pending save aborted that submission. Its eventual response did not replace the newly loaded deployment's instructions.
- A failed initial status read displayed its retry control; retry recovered the saved instructions. A failed account/DNS check focused its error with a 3px outline; a subsequent successful check focused its result.
- A signal-aware stalled check reached the real eight-second timeout, re-enabled the form and focused the retry explanation.
- The error state had no document overflow at CSS viewports 320×568, 568×320, 375×812, 768×1024 and 1440×900. Inputs had labels and the focused error retained its outline at each width.

Private evidence is `.local/issue-audit/handle-context-browser.json`, with source hashes recorded after qualification. These checks do not prove real DNS propagation, native account ownership, physical-device behavior, screen-reader speech or the complete onboarding journey. No real domain, tenant or production service changed. Issue #24 remains open for its wider acceptance criteria and dependencies.
