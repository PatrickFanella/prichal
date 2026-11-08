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

## Live staging-02 enrollment — November 8, 2026

All times UTC.

1. **Before state.** Registry dump `4760470e…` restored all 70 tables and every sequence into a disposable copy, which was dropped. `e4067ea1` was ready with one placement.
2. **Agent.** `node-agent` built from merge `83c645d37fcfb319b56d3a1563beb23213c459bc` with Go 1.27.1 (`dbf53b0884b04edaba8a052e7f4bd7084d3dcfe29651e4081bdcb3dc90038091`) replaced `08a1e03e…`, which is kept as `node-agent.before-managed-enrollment`. The drop-in `prichal-agent.service.d/managed-enrollment.conf` sets both opt-ins. `/var/lib/prichal-agent/managed-mail.json` (owner `prichal-agent`, mode 0600) reuses the existing Brevo SMTP relay credential with sender `info@subcult.tv`.
3. **Replacement, 01:44–01:45.** The tenant `68077cf0…` and edge `dd38b7c5…` were stopped and renamed with the `-before-managed-enrollment` suffix, and the edge `Caddyfile` was moved aside. One reconcile through `prichal-staging-02-managed-resume.service` created tenant `d5eb3c0d…` (mail, real PLC), guard `d7fff4e6…` on the managed tenant network and edge `8319d25d…` with `forward_auth`. The data and blob volumes, credentials file and TLS identity were unchanged, so the public route needed no republication. Public health returned 200, OAuth metadata 200 and `subscribeRepos` 101; signup without a lease returned 403.
4. **Enrollment, 01:45–01:47.** A fixed operator harness locked the registry binding, checked custody and called `Enroll` over worker mTLS with ticket `49aac16a-2663-4eb4-a95d-4c5c3029506e`. It wrote the invitation code to an owner-only file. Public `createAccount` with the owner's email created `did:plc:eqtyxf4i56sfx4plgd2necjd` as `at-49aac16a26634eb.subcult.tv`, and `requestEmailConfirmation` returned 200 from `info@subcult.tv` through Brevo. `plc.directory` holds one creation operation (01:47:02) with that handle and the managed PDS as service endpoint. `listRepos` shows the active repository; a second signup returned 403. Replaying the ticket returns `account_created`.
5. **After state.** Registry dump `2b6c906c…` restored all 70 tables; the deployment is still ready with one placement. The password, invitation and session files were deleted.

Two findings:

- Cloudflare answers the default `Python-urllib` user agent with an empty 403 before the edge. Operator clients must send their own user agent.
- The planned handle form `at-<32 hex>.subcult.tv` exceeds the PDS 18-character limit (#245). The account uses `at-` plus 15 hex characters of the ticket.

Email confirmation by the owner, handle publication and posting through the Bluesky app remain. Custody is `~/.local/state/prichal-staging-02-bootstrap/managed-enrollment-20261108/` on Kvant.

## Limits

Enrollment is driven by an operator over worker mTLS; there is no owner-facing managed enrollment flow yet.

## Handle routes

`managed-route-admin bind-handle --deployment UUID --node UUID --invitation UUID` records a default handle (schema 43, `managed_handle_bindings`) only when both sides agree:

- the native PDS, read through the enrollment ticket replay over worker mTLS, reports the invitation's generated handle `at-<15 hex>.subcult.tv` for its first account;
- the public `plc.directory` document for that DID lists `at://<handle>` first in `alsoKnownAs` and names this deployment's public endpoint as its PDS.

Replay returns the stored binding without reverifying. Each invitation, hostname and DID binds once. `export` then includes the deployment's bindings as inventory handles, and the renderer publishes each as an exact `/.well-known/atproto-did` proof route answering the DID. The Almaz publisher accepts handles in this exact form and refuses an update that drops or remaps a published handle. Handle withdrawal needs its own reviewed procedure.

Bindings are not rechecked after they are recorded. If an account later changes its handle or moves to another PDS, its binding needs an explicit review.
