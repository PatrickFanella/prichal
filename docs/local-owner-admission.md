# Local management-owner admission

Schema 27 adds a durable policy for creating management owners from a verified OIDC issuer/subject. Pausing it prevents a new owner and session from being created. Existing owners can still sign in and use their scoped access. It does not pause new native deployment requests, first-account requests, native account registration or issued PDS invitations. Those require their separate controls. Existing sessions and CLI access are not revoked by an owner pause.

After token verification, callbacks first look up/update an existing owner using the exact issuer/subject. Only new-owner creation acquires the policy's shared row lock, which remains held through owner/session commit. Closing policy takes the exclusive lock and waits for admitted transactions to finish. A subject committed by another callback during closure can still sign in as an existing owner. Missing policy denies new creation. Recovery holds deny session creation for both new and existing owners.

Effective operator changes and their audit commit together. Same-state retries add no audit or timestamp change. Actor/reason are bounded; no arbitrary customer text is accepted. An audit failure rolls back the policy change. Status returns only open state/update time, not the operator identity.

## Local commands

```sh
./scripts/dev.sh owner-admin status
./scripts/dev.sh owner-admin set --state closed --reason local_qualification
./scripts/dev.sh owner-admin set --state open --reason local_qualification
```

The command requires explicit local mode, a numeric loopback connection and `pds_local`. It refuses production/remote/customer databases. Its audit actor comes from the local OS identity. Accepted reasons are `maintenance`, `safety_review`, `capacity_review` and `local_qualification`. The migration preserves prior owner-creation behavior by initializing the local policy open; it grants no hosting entitlement. Live signup opening and production operator administration remain release gates.

## Verification and recovery

Full smoke and focused race tests passed. Tests cover paused OIDC callbacks without owner/session creation, existing issuer/subject sign-in, reopen, missing policy, validation, effective/repeated audit behavior, audit rollback, exclusive closure waiting for admitted commit and held recovery callbacks without metadata/session changes. Changed policy functions have 81–100% statement coverage; the broader auth package has 77.5% and local command test coverage 29.5%. Actual local commands are qualified separately. `golangci-lint` remains unavailable; no global tool was installed.

Actual drill `3b114b18-5b38-4463-a961-51b7cacc254d` completed at 2026-10-01T20:45:33Z. Alice/Bob sign-in and existing server scope remained correct during closure. Repeated close/reopen added no duplicate audits. Two effective audits remain and owner admission is open again. Owner/organization/deployment/job/enrollment/export/choice work and native data/custody matched the baseline. Sign-in intentionally changed authentication activity. New-subject denial is disposable callback-test evidence, because both retained source fixture subjects already exist. No source owner was removed to manufacture a new subject.

Management writers stopped for schema update and the fresh checkpoint. Recovery initially stopped before backup because the former inventory query exceeded PostgreSQL's 100-argument function limit at 51 tables. The query now aggregates trusted table entries and preserves its original private row-hash separator. A 50-table subset matched legacy counts/digests, and all 51 current inventory tables succeeded. An intermediate held copy remains retained; the final checkpoint follows that parity check.

Final recovery `c7ae795d-df96-425b-b67a-2644024abd6f` matched the 51 management/River inventory tables and all sequence positions, revoked copied authentication and refused held worker startup. Reconciliation/dedicated replay preserved default jobs, source rows/sequences, native signed records/media and protected custody. Both new policy/audit tables were included. API, worker and monitor resumed on schema 27 with fixture permissions disabled and owner/deployment/enrollment policies open. Actual readiness reports schema 27. Final post-recovery full smoke passed.

Older snapshots and native resources remain retained. No down migration or table removal is provided. Reverting to a schema-26 binary fails its newer-schema guard; use a forward correction or an isolated recovery copy rather than deleting policy/audit data. Independent storage, host-loss recovery and live signup qualification remain open under issues #25/#26.
