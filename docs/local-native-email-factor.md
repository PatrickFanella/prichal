# Native email sign-in codes

Issue #153 uses the pinned official PDS's email factor. Prichal explains it after first-account enrollment but does not enable, disable or inspect it through the management session. Management OIDC and native account sign-in remain separate.

## Owner setup

In a compatible Bluesky client, open Settings → Privacy and security → Two-factor authentication, confirm the native account email and enable email sign-in codes. The current menu and toggle are visible in [Bluesky's settings source](https://github.com/bluesky-social/social-app/blob/3d39ca0c0c0e9e22c72161885254efa2ef8a66e4/src/screens/Settings/PrivacyAndSecuritySettings.tsx). Use the native account credentials in that client or its PDS sign-in page. Prichal has no password/code input, native-session forwarding or factor mutation control.

The owner must keep access to the confirmed mailbox. Test a fresh sign-in in the chosen client before relying on it. Existing sessions remain active, and native app-password sign-in is exempt from the email challenge in the pinned PDS. Review app passwords separately; enabling the factor is not a session/app-password revocation action.

Factor status is **unknown** in management. Native `getSession` exposes `emailAuthFactor` only to an authenticated native account; an anonymous read is refused. Prichal does not acquire a native session to populate a badge and does not treat confirmed email as proof that the factor is enabled.

## Lost mailbox and support authority

Keep an existing signed-in native client open while recovering the original mailbox through the email provider. An active session can preserve current account access, but does not replace the mailbox verification required to disable this factor. The pinned PDS refuses a token-free disable request and leaves the factor enabled.

If mailbox recovery fails, request a hosting support incident using the established management account. Support may investigate delivery and explain the available recovery material; it must not ask for the account password, sign-in code or session token. A management owner/session, matching email address or claimed DID alone does not authorize factor removal. The current restricted agent and support APIs expose no factor reset or mailbox-change operation.

There is no qualified automatic lost-mailbox recovery path. Exceptional native account/email repair would require a separately reviewed operator procedure, independent ownership verification, explicit owner authorization, a protected recovery checkpoint and an audit of the exact action. Do not edit native SQLite, invoke an administrative email change or disable the factor silently through routine support. Such a change can remove protection and needs its own recovery evidence before being offered. If neither mailbox access nor a supported recovery path is available, a fresh primary-password sign-in can remain unavailable; Prichal does not promise a bypass. Key custody and live support procedures remain under #11/#22.

## Disposable qualification

```sh
./scripts/pds-local.sh qualify-email-factor
```

Each run allocates fresh labeled containers/volumes, private credentials and a dedicated internal Docker network without published ports. The PDS uses the repository's official image digest, isolated `.test` PLC identity and captured `.invalid` SMTP mail. It never opens the retained local fixture or live PDS, changes an existing account, sends external mail or connects to management PostgreSQL.

The command creates one native account, confirms captured email, enables the factor through authenticated native `updateEmail`, then checks:

- Password-only `createSession` returns 401 `AuthFactorTokenRequired` and no session.
- The sign-in mail is captured, its code completes login, and invalid/consumed codes yield no session.
- Native session state reports the factor enabled; an app password can still log in, and the probe revokes that app password afterward.
- A disable request without its mailbox token is refused and retains the enabled factor.
- Anonymous native state reads and mutation attempts return 401.
- The allocated containers' stdout/stderr contain none of the generated passwords, invitations, session tokens or captured confirmation/sign-in/disable codes.

Mail capture necessarily stores private test messages in the fixture database. Credentials and runtime state remain in private ignored `.local/pds-email-factor/<UUID>/` and private Docker volumes; no codes/tokens enter the JSON report. No management jobs, exports or audits are used. The log scan covers these allocated fixtures and generated values, not every possible upstream log or deployment. Success/failure and handled interruption stop allocated long-lived containers; reports retain any stop failures rather than hiding them. Retained volumes and stopped containers are not pruned.

## OAuth and live-mail limits

The pinned image's PDS source maps OAuth `emailOtp` to the same native account-manager login method (`dist/account-manager/oauth-store.js`), and its provider accepts an optional `emailOtp` sign-in field. That is source evidence, not a completed provider browser journey. The OAuth page/CSRF/device-cookie flow and authenticated third-party client UX were not exercised. Issue #153 explicitly permits documenting this gap. No gatekeeper proxy, TOTP component or ingress change was added.

Real sign-in-code delivery, mailbox-loss recovery, provider-page interaction, expiry timing and production enablement remain unqualified. The existing live pilot's prior email confirmation is separate from this captured-code test. Production onboarding still depends on the approved mail delivery and hosting gates; this change turns on no factor for a real account.

## October 16, 2026 evidence

Two native Linux Kvant runs passed using host Node 24.15.0 and official PDS digest `95d6179bc9fb10cc36ccd6dea0407fc6d99f21374bf2a1f9e8f28c0427c7e336`. Final private report: `.local/pds-email-factor/0a3e350a-05c6-4368-9465-50c53a434f8e/report.json`; earlier successful run: `40a767ef-88ba-40b1-a6f7-acdf457af69c`. Both stopped every allocated container with no stop failures. An initial request used an unsupported JSON body on an input-free endpoint; its failed report and stopped fixtures remain separate.

The actual `FirstAccount` component was checked with eleven controlled response states. Completed public/local accounts display guidance; denied, missing, unavailable, malformed, foreign or pending accounts do not. Even an injected factor flag leaves the displayed status unknown. No password/code form, native mutation or new management request was added. Authorization loss on polling hides the guidance and successful reads restore it. Width checks at 320, 667 and 1280 CSS pixels found no horizontal overflow. These are controlled browser-response checks, not a live signed-in owner journey. Private polling evidence is `.local/email-factor-browser.json`.

The full pinned `./scripts/dev.sh smoke` gate passed against separate `prichal-storage-qualification` PostgreSQL: 44 Node checks, frontend production build, Go vet and all Go race packages. Node/shell syntax and whitespace checks passed. The incidental package-manager lockfile edit was removed. Hosted CI, merge and deployment remain separate evidence.
