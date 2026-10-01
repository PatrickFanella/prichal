# Local service suspension and resume

Service operations are local operator actions. They record the OS operator, deployment, fixed reason, prior state, monotonic generation and result in management PostgreSQL. The intent, audit event and River job commit together. Customer routes cannot mutate lifecycle state. The API has no host credentials. The explicit local operator policy now permits fixed browser actions through the [support workspace](local-operator-workspace.md).

```sh
./scripts/dev.sh service-admin request --deployment INSTANCE_UUID --key REQUEST_UUID --state suspended --reason maintenance
./scripts/dev.sh service-admin status --operation OPERATION_UUID
./scripts/dev.sh service-admin request --deployment INSTANCE_UUID --key NEW_REQUEST_UUID --state running --reason maintenance
```

Commands require explicit local mode and the loopback `pds_local` database. Reasons are `maintenance`, `safety_review` and `resource_pressure`. Reuse the same request UUID after an uncertain response. A different payload or operator conflicts. A pending operation prevents another submission. Failed operations preserve the unknown host state; investigate before submitting a new generation. No automatic resume follows a failed suspension.

The worker uses the registered node's CA, certificate identity and public-key pin. It sends only the instance UUID, operation UUID, generation and desired state. The agent validates every retained runtime, ingress and registration guard before changing anything. It writes a private durable hold before stopping ingress, guard and PDS. Reconciliation and enrollment cannot undo that hold. Stop failures retain a pending lease. Resume starts only retained policy-validated containers and confirms local health before publishing success. Stale generations and changed same-generation requests fail.

No volumes, accounts or keys are deleted. Suspended instances still consume capacity. Owner responses show the lifecycle and fixed reason, hide operator identity, and suppress enrollment controls while held. Monitoring distinguishes a confirmed suspension from a runtime outage. It preserves unavailable usage counters. Management recovery requires the host's exact confirmed generation and operation, and retains its hold for pending, failed or mismatched lifecycle state.

The actual two-tenant local drill completed at 2026-10-01T09:36:24Z. Bob's three processes stopped, native serving became unavailable and ordinary reconciliation left them stopped. Alice remained available. Bob resumed in the original containers, with unchanged credentials, TLS identity, admission policy, confirmed account, signed repository and referenced media. An older suspension request failed. Customer escalation and foreign-owner reads returned 404. Report: `.local/service-lifecycle/f01c57c5-3eba-421d-a4a5-30e4ea88f96a/report.json`.

Managed recovery now captures an existing private service lease. A subsequent encrypted cold restore of resumed Bob completed at 2026-10-01T09:37:55Z, including 16 files, five SQLite databases and the running lease. Its four signed records and referenced blob survived; fresh writes and recovered PLC signing passed. Source and restored writers remained fenced. Report: `.local/managed-recovery/f3c0b25d-a048-445d-9aad-2a80110658c8/report.json`. This restoration drill refuses a suspended or pending lease and does not qualify restoration or export while held.

Native account takedown and held-account restoration now have [separate local evidence](local-held-recovery.md). Issue #17 remains open for service-held restoration, independent export/backup custody, customer notifications, dedicated operator policy and live qualification. Public DNS, TLS, mail and federation remain unqualified. The local browser automation host was unavailable; new lifecycle UI behavior has build and API evidence only.
