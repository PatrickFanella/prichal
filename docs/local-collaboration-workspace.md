# Browser collaboration rehearsal

The local owner workspace now includes invitation, shared status, membership and empty-organization ownership controls when the collaboration API is explicitly enabled. Default startup hides this section. Production rejects the underlying rehearsal opt-in. [The API permission contract](local-organization-access-design.md) still applies; no existing native/account, export, billing, host or CLI authority is widened.

Every mutation first retains a typed, owner-scoped intent in session storage. The review shows the exact identity, organization, role or ownership decision before a separate acknowledgement and submission. Reload restores the same action with acknowledgement cleared; it never submits automatically. Invalid retained data blocks new actions until explicit reviewed reset. Invite and ownership offers retain their stable request UUID. Acceptance addresses the existing identity-bound offer. Rename and membership requests retain their desired state and record effective changes only.

The interface cancels stale reads, aborts work on unmount and remounts separately for each management identity. Review and receipt headings receive focus. Viewer controls expose shared status; administrator controls add rename; owner controls add invitations, membership changes and eligible ownership offers. Invitation receipts and audits are shown as history, with current membership governing access. Recipient ownership acceptance is a distinct decision.

## Actual local browser evidence

The T3 collaborative preview can execute DOM inspection and focused interaction even though snapshot capture fails. Qualification used those native tools, not an alternate browser. Metadata evidence remains in `.local/collaboration-browser-functional.json` and `.local/collaboration-browser-widths.json`.

The browser rehearsal completed at 2026-10-01T16:05:05Z. Alice created empty organization `a982ab3f-19d0-4bc6-a6b1-7fc7a16c193c`, staged a viewer invitation, reloaded and confirmed no automatic submission. The access audit count stayed at eight until explicit confirmation. Tab, Space and Enter navigated review, acknowledgement and submission; successful receipt cleared the retained request and focused its status. Visible controls had associated labels.

A separately staged Alice rename remained retained across sign-out. Signing in as Bob hid Alice's retained action. Bob accepted the viewer invitation; rename/invitation controls were absent. Alice's next sign-in restored her own rename with fresh acknowledgement required. She then renamed the organization, promoted Bob to administrator and offered ownership. Offering left Alice as owner. Bob's administrator view allowed rename and hid invitation controls. A separate recipient review and acceptance made Bob the owner. An authenticated Alice request afterward returned 404.

A brief management-API outage during a retained rename produced an unconfirmed outcome. The browser kept the action, cleared acknowledgement and disabled confirmation until fresh review. After API recovery, explicit retry added one effective rename audit. No native hosting container was stopped for this check.

Viewer status and owner review/forms had zero document overflow and zero offscreen collaboration elements at 320×568, 375×812, 812×375, 768×1024 and 1440×900. Checks used actual `scrollWidth`, viewport and element bounds. These are DOM layout and keyboard results. Screenshot appearance and assistive screen-reader testing remain unqualified.

Both native tenants' signed repositories/media, container identities and protected credentials/TLS/admission state matched the pre-browser capture. The test organization is retained with Bob as owner; invitation/role/handover/rename audits are preserved. A default restart hid collaboration controls while the ordinary owner workspace remained available. Full smoke and the final frontend build passed.

Delegated administrator hosting/domain actions, full list pagination, populated handover, outbound mail, broad browser failure/accessibility coverage and live qualification remain open. This rehearsal does not close a milestone.

Fresh recovery `b3a05f17-fa9f-4599-85af-4b5d39dab6de` matched all 43 schema-23 tables and sequence positions after the browser work, including three invitations, three retained membership rows, two transfers and 16 access audits. Restored authentication was revoked and held worker startup refused. Host reconciliation and dedicated-queue replay preserved copied default work, source rows/sequences, native signed data/media and lifecycle/admission custody. All recovery copies remain retained.
