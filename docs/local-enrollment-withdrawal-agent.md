# Restricted first-account invitation withdrawal

The local node agent accepts `POST /v1/instances/{id}/enrollment/withdraw` with only a retained enrollment `ticket`. The registered worker's mTLS identity is required. The API has no host credential or Docker access. This document records the restricted command and typed worker client. [Owner intent and worker processing](local-enrollment-withdrawal-intent.md) follow this agent step; browser controls remain open.

An exact retained ticket and guarded local instance are required. A mismatched ticket cannot close a newer invitation. The command checks the sole-account inventory first. An existing account returns `account_created` or `verified` without changing its lease or restarting containers. The command never deletes an account, changes its identity, suspends service or issues a replacement invitation.

For an empty PDS, the agent publishes and confirms a `withdrawing` guard lease without a code. It drains ingress before the native process, then starts native and ingress. Draining ingress removes proxy requests authorized before guard closure. The fixed helper checks account inventory, disables unused native invite codes and checks inventory again. If signup completed during the drain, its account is preserved. Otherwise the agent publishes `withdrawn` before confirming completion.

An interrupted drain or unconfirmed helper response retains closed admission. Retry uses the same ticket. Confirmed withdrawal does not repeat the drain or mint an invitation. Enrollment cannot reopen that ticket; a separately authorized recovery ticket is required. A suspended instance refuses withdrawal rather than resuming service. The 25-second RPC deadline can interrupt a slow drain, so callers must treat errors as unconfirmed outcomes and retain the same ticket.

## Local evidence

Agent race tests cover mismatched and unsafe references, interrupted ingress drain, a lost helper response, retry, same-ticket reopening denial, idempotent completion, ordered ingress/native drain and signup completing during the drain. Contract tests reject extra action/DID fields, trailing data and missing worker authentication. Registered-node pin tests reject foreign peers and malformed results, including a withdrawal response containing an invitation.

The fixed JavaScript helper tests disable unused codes without replacements, preserve an existing account and fail closed on unavailable inventory/revocation or multiple accounts. They use injected SQLite/HTTP fixtures, not native runtime proof. The initial Go regression could not compile before the command existed; it was not a behavioral red test.

The full `./scripts/dev.sh smoke` gate passed. Focused race coverage is 80.4% for Docker withdrawal, 83.3% for account-result validation and 80.6% for the shared enrollment RPC client. Broader node-agent package coverage is 62.0%. Optional `golangci-lint` remains unavailable.

Actual retained-account drill `dd99de2e-2b43-43c5-b88f-ad3b6d459df0` completed at 2026-10-01T21:11:22Z using the updated agent and existing local PKI. Both native accounts remained verified through four repeated withdrawal-status calls. Foreign tickets and unsupported authority were denied. Native signed-record/media inventory, container IDs/start times, lease/credential/service/account custody and management deployment/enrollment/job state matched the baseline. No container restarted during those calls. The source management schema remains 27.

Run the optional preservation drill from the repository root:

```sh
node scripts/qualify-enrollment-withdrawal-preservation.mjs
```

It accepts only the two retained disposable local account instances and requires their completed leases. Private report metadata goes to ignored `.local/enrollment-withdrawal/`. It cannot qualify successful withdrawal of an empty PDS. Existing tenants, volumes and recovery copies remain retained; no slot was freed for a fresh attempt.

## Remaining work

The management workflow now persists owner intent, audit and an atomic worker job with original/current scope, recovery checks and issuance/refresh serialization. Add browser review, retry and account-completed guidance. Preparing-request cancellation remains separate because this command requires an exact retained ticket.

A fresh empty native PDS must qualify real ingress drain, unused-code rejection, restart failure and signup/withdrawal races. These remain runtime gates, alongside production agent policy and live deployment qualification. Existing-account preservation and fixture tests do not complete issue #26 or open public signup. Older agents reject the new RPC; do not enable customer withdrawal until the designated agent version is installed. Older enrollment code cannot reopen the newly closed lease states.
