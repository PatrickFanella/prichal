# Local operator support workspace

Schema 20 adds audited support inspection. Operator rights are independent of organization ownership. A private local policy binds an exact OIDC issuer and subject to `inspect` or `operate`; email and display name never grant access. The policy is loaded at API startup. Removing or changing a role requires restarting the API. Every request validates the current management browser session and matches its verified owner identity against that policy. CLI credentials cannot enter the operator namespace.

By default, operator routes return 404 and the browser support section stays hidden. `PDS_LOCAL_OPERATOR_FILE` enables only explicit local qualification. Production startup rejects this local policy. No production operator identities or permissions are configured.

## Authorized behavior

An operator can inspect an exact organization or instance UUID. Organization lookup returns up to 100 safe instance references and explicitly marks truncation. Instance inspection projects deployment state, hostname, domain proof, observed usage, fixture billing state, local grant state, incidents and current lifecycle intent. An audit row commits before the data is returned; failed audit persistence prevents disclosure.

Lifecycle history has pages of 50 operations with an explicit generation cursor. Operator history includes recorded actor attribution. Customers can read only their own deployment's lifecycle history, with actor identities replaced by `operator`. Native credentials, browser/CLI tokens, private records, invoice bodies, email addresses, idempotency keys and raw host error text are excluded.

The `operate` role can submit only fixed service/account suspend or resume intent with a stable UUID and one of the existing reasons. Browser origin and CSRF checks apply. The lifecycle store commits actor, target, prior state, intent, initial audit and River job together. The existing restricted worker and node agent execute it. The API receives neither Docker access nor native/host credentials. The inspect-only role cannot mutate lifecycle state, and payloads cannot claim another actor or role.

The browser asks the operator to review the target and current activity before submission. It retains the request key/payload in owner-specific session storage before the network request. Reloading never submits an action automatically. An uncertain response can retry that request; a new action requires explicit reset. Inspection does not borrow a customer session or impersonate the customer.

## Local fixture

Create an ignored `.local/operators.json` with mode 0600. The disposable issuer uses subjects `alice` and `bob`. A qualification policy may contain:

```json
[{"issuer":"http://localhost:5556","subject":"alice","role":"operate"}]
```

Restart this project's API with the explicit policy; use the ordinary worker with its existing restricted-agent mTLS environment:

```sh
PDS_LOCAL_OPERATOR_FILE="$PWD/.local/operators.json" ./scripts/dev.sh api
node scripts/qualify-support-workspace.mjs \
  .local/owner-enrollment/ALICE_JOURNEY_UUID \
  .local/owner-enrollment/BOB_JOURNEY_UUID
```

The qualifier verifies exact operator authorization, Bob's denial, organization lookup, customer history scope/redaction, CSRF rejection, idempotent account takedown and separate audited resume. It preserves both tenants' signed repositories, referenced media and protected custody. Restart the API without the policy afterward. No operator role is written into customer membership tables or granted implicitly to existing owners.

## Evidence and remaining gates

Full smoke verifies private policy loading, issuer/subject separation, inspect-only restrictions, queue-backed fixed actions, stable replay, response redaction, atomic inspection audit failure and history pagination. The actual native fixture drill completed at 2026-10-01T12:23:34Z. Generation 21 suspended Bob's account, and generation 22 resumed it through Alice's explicitly authorized management session. Alice remained available and both tenants retained data/media/custody. Private report: `.local/support-workspace/8b27f0b9-3199-44d6-aa24-58c6c57c2e15/report.json`.

Collaborative browser snapshots still fail after reopening the page, so responsive layout, keyboard/screen-reader operation and browser action persistence remain unqualified. The frontend compiles. Dedicated operator identity deployment, MFA/access review, role revocation procedures, incident/review notifications, deployment retries, backup scheduling and live support qualification remain open for #22. Recovery views report management state and existing monitoring evidence; manual local restore reports are not a scheduled backup guarantee.
