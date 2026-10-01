# Local first-account enrollment

A signed-in owner prepares a one-use invitation from their ready deployment. The API authorizes the organization, records intent and a River job in one transaction, and returns a retryable operation. Only the worker contacts the authenticated restricted node agent. The API has no Docker access or node credentials.

The agent issues native PDS invitations behind a tenant-bound fifteen-minute admission lease. Recovery closes admission, drains pending registration, revokes unused codes and publishes a replacement. A repeated operation preserves its code and original expiry. Native repository inventory closes registration after the first account independently of management refresh.

Management stores invitation codes under AES-256-GCM with operation, deployment and organization identifiers as authenticated scope. `scripts/dev.sh` prepares a private random 32-byte `.local/enrollment.key` without overwriting it. Preserve this key separately from management backups; losing it prevents reading outstanding invitations. Passwords, email-confirmation tokens and native session tokens go directly to the PDS. The browser stores only the retry request identifier and recovery flag, and clears displayed codes when authorization fails or the invitation expires.

The native local account page is linked from the deployment. Fixture handles use `.test`, with 3–18 characters before the suffix. Mail uses `.invalid` and stays in the captured local inbox. The per-instance certificate remains private fixture trust; no system root is installed. Native browser navigation may require fixture trust. CLI qualification trusts only the exported instance certificate.

To qualify a fresh, empty, Alice-owned local deployment:

```sh
node scripts/qualify-owner-enrollment.mjs DEPLOYMENT_UUID
```

The command creates a retained disposable account and stores its credentials only in a private `.local/owner-enrollment/UUID/account.json`. It checks owner replay, foreign-owner denial, invitation recovery, native creation and login, first-account admission closure, captured email confirmation and management's verified checkpoint. Its secret-free report is retained alongside the credentials. An interrupted drill can resume with its private journey directory as the second argument; review the retained state before retrying. It never resets populated storage.

Regression tests cover concurrent submission, changed-payload replay, owner/CSRF boundaries, three requests per hour, code scope and tampering, an uncertain native result after a failed management checkpoint, and recovery holds. Production enrollment remains disabled pending live domain, issuer, mail and identity qualification. These local checks do not prove public DNS, TLS, email delivery or downstream federation.
