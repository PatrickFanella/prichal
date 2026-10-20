# Disposable native member deactivation

Issue [#117](https://git.subcult.tv/subculture-collective/prichal/issues/117) requires a member to keep working during a migration window and then be deactivated rather than deleted. This delivery qualifies the native operation needed at the end of that window. It does not yet connect it to an agent route, management request, timer or browser removal control.

## Fixed native operation

`backend/internal/nodeagent/member-exit-helper.cjs` supports only `native_status DID` and `deactivate DID`. It runs inside a disposable identity-enabled PDS and requires the literal local PLC fixture URL. The native administrator password remains inside that runtime. The helper reads SQLite in a read-only transaction, refuses the earliest owner account, unknown DIDs and accounts without recorded invitation use, and verifies the returned subject identity.

For an active member it calls only `com.atproto.admin.updateSubjectStatus` with a repository subject and `deactivated.applied=true`. The pinned PDS implementation calls its account manager without a deletion deadline. The helper refuses an existing takedown or unexpected active-account deletion policy, then independently confirms native status and the retained actor row. It never activates an account, clears a takedown or schedules deletion. An already-deactivated member is returned without mutation, preserving a user-selected deletion schedule and original deactivation timestamp. Failed or uncertain writes return no success result.

The read action is named `native_status` because Node treats `inspect` as its debugger command even when supplied after `-e`. Fixed callers must use the allowlisted action.

A future restricted host broker must resolve the DID from its retained accepted-invitation journal and enforce the migration deadline before invoking this primitive. The helper itself does not know the membership policy or elapsed migration window and is not exposed through the agent HTTP handler or management API. This qualification does not authorize immediate customer-account removal.

## Qualification command

```sh
./scripts/pds-local.sh qualify-member-exit
```

This command allocates a new labeled internal Docker network, three resource-bounded containers and independent data/blob/protocol volumes. It publishes no ports, uses the pinned official PDS and Caddy images, and creates three synthetic accounts against the local PLC/mail fixture. It does not open the retained local fixture or a management database. Credentials and reports stay under ignored `.local/pds-member-exit/UUID/` with private permissions. Its finalizer stops only containers with the exact qualification label; it retains volumes and reports, including failures.

On Kvant, October 19, fixture `56365f97-9914-4b91-a952-7ec38e60a35b` passed:

- Owner and unknown-DID requests were refused before mutation.
- A member created a native post with a PNG blob before deactivation. The actor row stayed present with no deletion deadline; the owner row was unchanged.
- Logical actor-store SQLite content, signing-key files and blob bytes matched before/after deactivation and restart. The seven-file inventory hash was `1d9360163280d27dd397e55a24783a67fa1ddec9404308bf2bc8a08c4dbf9024`. SQLite tables are read transactionally so checkpointing does not create false file-byte differences; this comparison is not a backup/restore qualification.
- A write with the member's existing session returned HTTP 401 identifying deactivation. Replaying deactivation retained the timestamp.
- A separate member's existing user deactivation and future deletion schedule were unchanged.
- Native restart retained container identities and deactivation state, and owner login still succeeded.
- Generated credentials were absent from fixture logs. All three containers stopped, with no stop failures; their volumes remain retained.

The private receipt is `.local/pds-member-exit/56365f97-9914-4b91-a952-7ec38e60a35b/report.json`. Earlier failed fixture reports/volumes remain retained. Signup harness assumptions were corrected to short unique handles. The first read-action trial hit Node's reserved debugger command before member deactivation was attempted; the final action name is qualified above.

Three Node boundary tests also cover owner/foreign/uninvited/nonlocal refusal, arbitrary actions, preserved existing deactivation, policy conflicts, foreign status, failed writes and lost responses. Full pinned smoke passed with 50 Node tests, frontend build, Go vet and all Go race packages; the final renamed read action passed its focused tests and the native fixture. Shell and JavaScript syntax checks passed.

## Remaining #117 work

The host broker still needs a retained accepted-ticket binding and enforced migration deadline. Management still needs durable removal intent, audit, scheduling/reconciliation and browser review. The complete exit policy must cover organization hosting termination and handles under verified organization domains. Administrator/native end-to-end qualification, plan limits and production enablement remain separate. No issue is closed by this native primitive.
