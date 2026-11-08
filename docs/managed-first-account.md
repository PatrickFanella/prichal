# Managed first-account enrollment and mail

Part of #8. Generated managed deployments started with account creation closed at the edge and no mail settings, so they could not enroll a first account or send verification email. Two host opt-ins on the node agent now provide both, for generated managed instances only. Private fixtures and the pilot runtime are unchanged.

## Opt-ins

Both require `PDS_STAGING_MANAGED_RUNTIME=true` and are refused otherwise.

- `PDS_AGENT_MANAGED_MAIL_FILE` names an owner-only JSON file with exactly `emailFrom` and `smtpUrl`. It is validated like the pilot policy: a plain sender address, and `smtps://user:password@host:465` with no path, query or fragment. Managed PDS containers then get `PDS_EMAIL_FROM_ADDRESS` and `PDS_EMAIL_SMTP_URL`.
- `PDS_AGENT_MANAGED_ENROLLMENT=true` requires managed mail. Each managed instance then runs the first-account enrollment guard (`PDS_GUARD_FIRST_ACCOUNT_ONLY=true`) on its managed tenant network. The edge routes `createAccount` and OAuth sign-up through that guard instead of answering 403. Signup stays closed except during a 15-minute lease. The agent accepts `POST /v1/instances/{id}/enrollment` and `/enrollment/withdraw` for managed instances; every other managed mutation stays closed.

The in-PDS helper receives the generated hostname and refuses unless the PDS runs with dev mode off, `https://plc.directory`, the matching `did:web` service DID and SMTPS mail.

## Existing deployments

The agent compares a running deployment with its policy exactly, so a managed deployment created before these opt-ins (no mail, closed edge) is refused once they are enabled. To move it over:

1. Take a verified registry snapshot and record container IDs and credential hashes.
2. Stop the agent. Stop the tenant and edge containers and rename them with a `-before-managed-enrollment` suffix; they stay stopped with restart policy `no` as the rollback copy.
3. Move the edge `Caddyfile` in the managed edge directory aside with the same suffix.
4. Install the mail policy and opt-ins, start the agent and reconcile. The agent recreates the tenant on the same data and blob volumes and credentials, creates the guard and recreates the edge with the retained TLS identity. The namespace fence permits this only while the renamed containers are stopped.

The edge certificate and public route are unchanged, so no republication is needed. To roll back, stop the agent, remove the new tenant, edge and guard containers, rename the retained containers and `Caddyfile` back, and restore the previous agent environment.

## Limits

Enrollment is driven by an operator over worker mTLS; there is no owner-facing managed enrollment flow yet. Default account handles under `at-<invitation>.subcult.tv` are not published, because managed inventory exports carry no handle bindings yet.
