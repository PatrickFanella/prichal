# Export request context

Export status reads started before a submission can no longer replace that submission's result. A request sequence invalidates those older reads, and export-list polling waits while submission is busy. A synchronous guard also refuses overlapping submissions. Changing deployment or session context aborts pending submissions and clears the displayed export state before reading current access. The retry key is restored from that deployment's browser storage.

Failed status reads keep export controls paused and show the unavailable explanation without the initial loading message. Later successful polling restores access. The original retry key remains in storage until a validated submission result is confirmed; a context change preserves an uncertain request for its original deployment.

## Controlled browser evidence — October 20, 2026

On base `e9b646d`, the actual `CustomerExports` component reproduced the race: a confirmed submission displayed `Export queued`, then a delayed older empty-list response removed it. The private before/after receipt is `.local/issue-audit/export-context-browser.json`.

The revised component passed these checks in the T3 browser with its actual React code and styles and synthetic loopback API responses:

- The same delayed empty-list response did not hide the export confirmed by the newer submission.
- Switching deployment while another submission was pending aborted that submission. Its eventual response did not add an export to the new deployment or clear the original deployment's retained key.
- A failed status read displayed the unavailable explanation without `Loading exports...`. Subsequent polling recovered access and the original retry key.
- A lost response and a later confirmed retry used the same key. Confirmation cleared it and displayed the queued export. Another lost response and a keyboard retry also kept the same key and focused the error alert.
- The error state had no document overflow at CSS viewports 320×568, 568×320, 375×812, 768×1024 and 1440×900. The alert retained focus with the browser's 1px outline at every width.

The private receipt includes qualified source hashes. These desktop-browser checks do not qualify real archive capture, download integrity, native recovery, physical devices, screen-reader speech or the complete export/cancellation journey. No archive, account, tenant or production service was created or changed. Issues #21 and #24 remain open for their wider acceptance criteria and dependencies.
