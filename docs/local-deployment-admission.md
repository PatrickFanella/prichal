# Pausing new local deployments

Schema 24 adds a durable admission policy for new deployment requests. This control is separate from node capacity, hosting grants and service/account suspension. It accepts no host credential, changes no running PDS and deletes no storage.

The policy starts open to preserve the disposable local workflow. Production HTTP provisioning remains independently disabled. Opening this policy does not authorize production serving or public signup. The operator command requires explicit local mode and a numeric loopback `pds_local` database.

```sh
./scripts/dev.sh deployment-admin status
./scripts/dev.sh deployment-admin set --state closed --reason maintenance
./scripts/dev.sh deployment-admin set --state open --reason maintenance
```

Supported reasons are `maintenance`, `safety_review`, `capacity_review` and `local_qualification`. The command records the local operating-system actor, reason, prior state, requested state and time for each effective change. Repeating the current state creates no extra audit. The policy update and audit commit together; an audit failure leaves the prior policy intact. No owner or CLI management endpoint can change this policy.

New simulated and native deployment submissions check the policy within the existing organization-authorized intent transaction. Each accepted submission holds a shared policy lock through intent/event/River-job commit. Closing takes an exclusive lock, so the close command waits for previously admitted transactions to finish. Requests which reach admission after closing cannot create work. A missing policy or management recovery hold fails closed. Existing committed jobs continue; this is not a fleet stop command.

Stable retries first verify current organization ownership and compare the original name, region and kind. A matching request returns its retained deployment even while admission is closed. A changed payload still conflicts and a foreign owner still receives not found. Status, history, export and recovery paths do not depend on the admission switch. A rejected new submission receives HTTP 503 with instructions to retain its request key; retrying after reopening uses that same intent.

## Local evidence

The full smoke gate passed Node checks, frontend build and Go vet/race/PostgreSQL tests. Tests cover both deployment kinds, stable replay, owner isolation, changed payloads, concurrent effective changes, shared-lock serialization, audit rollback, missing policy, recovery holds and unsafe operator targets. Optional golangci-lint remains unavailable.

Actual drill `883cacc8-ba47-4123-b8fe-5566a631a34b` completed at 2026-10-01T16:45:09Z. Closing rejected new native and simulated requests while Alice's retained native request returned its original operation. Deployment status, hosting history and export reads remained available. Repeated close recorded no extra change. The separate reopen restored the initial open policy. Exactly two effective audits were added, while deployment/event/job/lifecycle counts and both signed native repositories/media, container identities and protected custody files remained unchanged.

Run `node scripts/qualify-deployment-admission.mjs` only against the fixed populated disposable local Alice/Bob fixtures. It requires admission initially open, retains private evidence under `.local/deployment-admission/` and restores the initial policy in a cleanup block. If the operator database is unavailable during cleanup, inspect policy status before proceeding.

Production operator authentication, public signup/enrollment controls, capacity budgets and signed-off release qualification remain separate work for #26. Existing registration and first-account enrollment are not disabled by this deployment-only switch.
