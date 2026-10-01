# Local native account moderation

Account takedown and whole-service suspension are separate audited operator actions. Schema 16 adds the lifecycle kind while preserving one durable operation order per deployment. Each kind retains its own prior and desired state. A pending operation prevents conflicting service or account work. The worker requires a verified first account before requesting native moderation.

```sh
./scripts/dev.sh service-admin request --kind account --deployment INSTANCE_UUID --key REQUEST_UUID --state suspended --reason safety_review
./scripts/dev.sh service-admin status --operation OPERATION_UUID
./scripts/dev.sh service-admin request --kind account --deployment INSTANCE_UUID --key NEW_REQUEST_UUID --state running --reason safety_review
```

The restricted agent derives the sole native actor from read-only account SQLite. Requests cannot supply a DID, password, host path, command or native administrator method. The fixed helper calls the pinned runtime's `com.atproto.admin.getSubjectStatus` and `com.atproto.admin.updateSubjectStatus` with a repository subject. Administrator credentials remain inside that tenant's PDS.

Before changing uncertain custody, the agent inspects native state. It accepts only a takedown owned by its retained applied reference or a prior pending platform operation. Other moderation references fail closed. Private leases retain generation, ticket, DID, phase and applied reference. Resume clears only takedown; it never sends user activation or deactivation fields. Native results and management checkpoints must match the requested operation and verified account. Read-only inspection confirms the protected fence against native status.

Customer responses expose account moderation independently of service lifecycle. They do not expose operator identity or native keys. Monitoring checks confirmed native moderation while the service is running. Recovery release refuses pending, failed, mismatched or uninspectable account state. A service held with existing moderation requires further operator recovery work because native inspection cannot run while stopped.

Actual local Bob qualification completed at 2026-10-01T09:55:14Z. Takedown denied normal login and writes from an existing native session with 401 `AccountTakedown`. Public repository serving returned 400 `RepoTakendown`. Operator-authenticated exports still verified signed records and referenced media. Alice remained available; customer escalation and foreign management reads returned 404.

The drill then applied an audited service hold, fenced all tenant writers, and captured native data, blobs, protected credentials, TLS identity, admission/ingress policy and both lifecycle leases. Its 435,712-byte cold archive passed authenticated encryption and decryption with an identical digest. Service resume retained the native account takedown. Its separate audited account resume restored confirmed login without changing containers, data, media or identity custody. An older account operation failed. Report: `.local/account-lifecycle/b7554d52-eda4-4260-9706-c2279451f773/report.json`.

The first drill failed before a native takedown. Node treated the fixed `inspect` argument as its debugger command even after an argument separator. The helper now uses `native_status`; an actual invocation and the corrected drill passed. The failed management operation remains audited with no native state change.

After resume, managed recovery captured both private lifecycle leases and completed at 2026-10-01T09:56:51Z. Seventeen files, five SQLite databases, five signed records and one referenced blob survived. Fresh writes and recovered PLC signing passed, then restored writers stopped before the original resumed. Report: `.local/managed-recovery/cb7c09eb-8b93-4ac1-b7a8-6ebbf0c815d3/report.json`.

The local gate covers native helper ownership, replay, read-only inspection, user-deactivation preservation, invalid actor inventory, private-error redaction, request restrictions, pinned access, independent lifecycle ordering and recovery fences. Actual user deactivation was not exercised. The cold capture is operator recovery material, not a customer export, and its small local key is retained privately for this drill. Held-state restoration, independent backup custody, customer notifications, broader operator authorization and production qualification remain open for #17. The new UI has build and API evidence; the browser automation host remains unavailable.
