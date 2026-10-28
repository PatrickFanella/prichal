# Private staging provisioning for #6

## Source behavior — October 28, 2026

The management API and worker now support an explicit private provisioning slice with `PDS_MODE=production` and `PDS_STAGING_PROVISIONING=true`. The default production API still refuses provisioning. Local simulation and its worker retain their existing behavior.

This slice uses production authentication rules, including HTTPS issuer/origin and secure management cookies. It accepts only explicit `kind: agent` deployment requests, retaining owner/CSRF checks, admission policy, active pilot grants, instance allowances and original-key replay. Simulation requests are rejected. No payment entitlement or public signup is enabled.

Ordinary private staging deployments enter the durable `staging_deployments` queue. The staging worker consumes only its staging queues, uses the existing recovery fence and authenticated agent reconciler, and refuses simulation jobs even if they are placed in its queues. It registers no billing, enrollment, member, lifecycle or export workers. Configured local billing/export fixtures or enrollment key settings are rejected rather than used in this worker. The separate managed-intent preparation queue is described below.

The worker requires the dedicated CA, certificate and private key before connecting to PostgreSQL. The API receives none of these agent credentials and no Docker access. Agent requests remain restricted to registered, key-pinned loopback HTTPS endpoints reached through the existing private tunnel arrangement; the node agent alone owns host execution.

## Configuration contract

Set these in private, separate API/worker environment files outside Git:

- Both: `PDS_MODE=production`, `PDS_STAGING_PROVISIONING=true`, and the designated staging management `PDS_DATABASE_URL`.
- API: the selected HTTPS `PDS_PUBLIC_ORIGIN`, HTTPS `PDS_OIDC_ISSUER`, OIDC client configuration and existing management authentication settings.
- Worker only: `PDS_WORKER_AGENT_CA_FILE`, `PDS_WORKER_AGENT_CERT_FILE`, `PDS_WORKER_AGENT_KEY_FILE`, referencing the dedicated enrolled worker identity.

Use a designated staging inventory with a verified registered node and an operator-authorized unoccupied slot. The retained registry and allocator still call this private qualification region `local`; endpoint allocation remains `<operation UUID>.localhost` with its stable reserved port. This is deliberately private ingress. It does not select a managed public namespace or complete #7/#126.

Do not switch an existing local fixture database to this path to move pending jobs implicitly: existing jobs retain their original queues. Drain them with their original supported worker or retain them for explicit reconciliation. Disabling this opt-in pauses new staging submissions and worker processing; it neither deletes retained jobs nor removes native resources. Existing authorized operation reads remain available.

## Verification and live gate

Automated checks cover opt-in parsing and default closure, mandatory transport before database startup, owner/CSRF/grant enforcement, simulation refusal, durable queue selection, stable identity/placement after uncertain host success and recovery fencing. A real PostgreSQL/River test completes an agent operation with a synthetic agent while leaving a default-queue simulation queued and creating no simulated runtime. This is an actual queue test with synthetic host behavior, not a native staging deployment.

Read-only inspection of Kvant on October 28 found the dedicated staging VM and tunnel active. Registered node `1ef77d45-c6d5-4bdb-b77b-ba1103b45a66` was active with one effective slot, one operator-authorized slot and one retained placement. No ceiling, service, tenant or VM was changed.

#6 stays open until a designated spare slot is available and the new control path passes the actual staging drill: create a native PDS, interrupt after host success, resume the original operation, verify exactly one native runtime/storage namespace and placement, and retain capacity/timeout/identity rejection evidence. Existing host-controller and local-mode evidence remains dated evidence. Neither this implementation nor green local smoke completes that drill.

See [the issue audit](issue-acceptance-audit-2026-10-28.md), [private staging connection](live-staging-management-connection.md) and [progress](progress/local-m1-alpha.md).

## Subsequent native qualification

The owner authorized the second node and it passed the [native provisioning/interruption and boundary drills](live-staging-02-provisioning.md). The original occupied node was preserved. This resolves the capacity block recorded above; source integration remains separate from local and native checks. The staged node agent also uses explicit production-mode staging opt-in while retaining private endpoint allocation. Real owner login, public DNS/TLS, enrollment and release qualification remain separate.

#6 subsequently closed after PR #202 merged with its required check passing. #7 is active. Its additional API-only `PDS_STAGING_MANAGED_INTENTS=true` opt-in prepares [atomic managed hostname intent](managed-routing-inventory.md#atomic-api-intent-preparation) with an empty serving URL and the separate `staging_managed_deployments` queue. Older private workers cannot consume that queue; the current staging worker snoozes it before host calls or placement. The ordinary private path above remains supported. The new opt-in was not enabled on the retained native fixture, and neither preparation nor a green source check establishes public hosting.
