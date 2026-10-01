# Product fit with Subcult OS and Patchwork

Research checked September 30, 2026. This is product discovery and read-only source inspection, not a runtime, security or deployment certification. Subcult OS was clean at d40221f00a9bb6790240e310cde1b3b2c6497b9d. Patchwork inspection uses public mirror commit 7498e47f2471b349e0f3fa2e8415f150b5e28ad9. Private-source parity is unverified.

## What is the product and what can the repositories contribute?

### Takeaway
The product is a managed creator or organization server on the customer's domain, with useful applications and ongoing operations included. PDS hosting is one component. Subcult OS supplies the strongest starting workflow; Patchwork supplies a possible later mutual-aid module.

### Cited findings
- Subcult OS documents an owner/workspace/invitation lifecycle, published event pages, free reservations, door check-in, closeout reports, workspace switching and public discovery. Its API source registers the corresponding handlers. This establishes source implementation, not a fresh execution result. [README](/Users/onnwee/Projects/subcult/subcult-os/README.md); [API routes](/Users/onnwee/Projects/subcult/subcult-os/backend/internal/app/app.go)
- Subcult OS separates private contacts, staffing, finances, attendance and consent from public cultural records. Its accepted brief explicitly excludes PDS provisioning and simultaneous Patchwork integration from the first consolidated release. [Development brief](/Users/onnwee/Projects/subcult/subcult-os/docs/development/development-brief.md)
- The current README says identity-only AT OAuth exists but remains disabled pending live qualification; publication is not implemented. It describes an optional projection worker. Older architecture prose still says projection is design-only, so source and newer documents must resolve this drift. [README](/Users/onnwee/Projects/subcult/subcult-os/README.md); [Architecture](/Users/onnwee/Projects/subcult/subcult-os/docs/development/architecture.md)
- The exact Patchwork project is PatrickFanella/patchwork, an AT-native mutual aid application with web, query/chat/volunteer APIs, indexing and moderation services. [Public repository](https://github.com/PatrickFanella/patchwork)
- Patchwork API chooses PostgreSQL for queries when configured, but initializes fixture auth, chat, volunteer, settings and verification services independently of that choice. Its organization portal stores profiles, membership, listings and audit records in memory. [Pinned API startup](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/index.ts); [Pinned organization portal](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/org-portal-service.ts)
- Patchwork has a tenant-service source file, but it explicitly implements an in-memory model for tenant boundaries, regions and failover. This does not establish actual provisioning, persistent tenancy or regional infrastructure. [Pinned tenant service](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/tenant-service.ts)
- Its default Compose includes web/API/Postgres/indexer/moderation services, but does not provide the indexer database URL that selects persistent mode. The indexer source selects a fixture pipeline without it. [Pinned Compose](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/docker-compose.yml); [Pinned indexer](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/indexer/src/index.ts)

### Inferences
- Start with independent venues, recurring event organizers, artist collectives and small cultural organizations. They share an immediate workflow already represented in Subcult OS. Creators who only need a website are a weaker first fit.
- Reuse Subcult OS applications and explicit data boundaries. Add a hosting service around them for customer/domain inventory, deployment configuration, secrets, updates, backups, metering, billing and support. Keep hosting administration separate from workspace business permissions.
- Use Patchwork as reference material for mutual-aid requests/offers, volunteer coordination and resource directories. Admit one funded journey at a time. Do not merge two identity systems or market fixture functionality as a supported service.
- Default to an isolated application installation and database per pilot organization on a shared managed fleet. This is different from promising a dedicated physical server or VPS. It limits initial application changes but still requires host-level controls. Compare a shared multi-tenant application only after measuring operating costs.
- Preserve customer ownership of domain and public identity. Keep private operational data in the application database. Export and recovery must cover both application data and PDS repositories when the latter are included.

### Gaps
- No current production runtime or customer deployment was inspected. No claim of turnkey commercial readiness follows from this source review.
- Licenses and rights for every application/dependency need review before offering hosted distribution. Hosting eligibility was not established here.
- Branding, domain routing, complete customer exports, provisioning, subscription billing and fleet recovery need explicit product requirements. Source search did not establish a complete hosting control plane.

## What does the customer buy and do?

### Takeaway
Sell a working organizational home that the customer can use without operating infrastructure. The first success is a real event managed on their domain, followed by another event, rather than a successful server install.

### Cited findings
- Official AT documentation distinguishes PDS data hosting from application hosting and recommends an app-operated PDS for direct newcomer signup. It does not say a PDS supplies the app itself. [Self-hosting](https://atproto.com/guides/self-hosting)
- Custom Bluesky domain handles can work on existing hosting; they do not require this service. [Bluesky domain guide](https://bsky.social/about/blog/4-28-2023-domain-handle-tutorial)
- AT production guidance requires distinct domain arrangements for PDS and application hosting, including avoiding same-site subdomain arrangements for OAuth/blob safety. A branded application domain and a domain handle need not equal the PDS hostname. [Going to production](https://atproto.com/guides/going-to-production)

### Inferences
Proposed promise: "Bring your domain. Launch your own home for your collective, events and community. We run the hosting, updates and backups."

Proposed pilot journey:
1. Customer chooses the event/collective package, identifies the owner and brings a domain. We disclose the application address, identity handle and any separate PDS endpoint.
2. We configure DNS/TLS, branding, application/database/media/mail and backups, then invite the owner. Pilot provisioning can be manual.
3. Owner invites their team, creates a public event and accepts free reservations. Team runs check-in and reviews the report.
4. We monitor services, maintain software, rehearse recovery and provide bounded support. The customer runs programming, membership decisions and community moderation.
5. Owner repeats the workflow. Later, qualified AT publication connects public cultural records across compatible applications; Patchwork adds a separately qualified community service journey.
6. Customer leaves with documented domain/DNS handover, application exports, media and any PDS migration assistance. A PDS export alone does not export private Subcult OS operations.

Initial package should include one organization, its public pages, team workspace, free event reservations/check-in, media allowance, transactional email allowance, updates, backups and stated support hours. Treat paid tickets, subscriptions, marketing campaigns, native app distribution and mutual aid as separately scoped extensions. Do not imply general website editing or arbitrary plugin hosting.

### Gaps
- Customer interviews must determine whether the paid problem is event operations, community coordination, publishing, or control of identity. These may have different buyers and price tolerance.
- A public federation promise needs a working publication and discovery journey. In the pilot, describe the current application functionality and the future protocol connection separately.
- Sharing network discovery across isolated customer installations needs an explicit permission and data contract. Domain isolation does not automatically create a connected network.

## Can this support a bootstrapped business?

### Takeaway
Test a paid, manually operated full-application pilot before building self-service. Model the entire hosted service and operator time. The existing PDS-only cost tables do not price this product.

### Cited findings
- Subcult OS requires a web/API/Postgres stack, media configuration and separately enabled email/background workers. [README](/Users/onnwee/Projects/subcult/subcult-os/README.md)
- Patchwork's deployment includes additional indexing/moderation workers as well as its application and database. [Pinned Compose](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/docker-compose.yml)

### Inferences
All numbers below are illustrative assumptions, not provider quotes, capacity results or validated willingness to pay.

Monthly contribution per customer = subscription revenue minus allocated application/API/database compute, PDS compute if included, media/storage/egress, transactional mail, worker/indexing costs, off-site backups, monitoring/security tools, payment fees and direct support/maintenance/recovery labor. Development, acquisition, tax and founder salary overhead remain additional costs.

Example full-stack pilot at $149/month:

| Assumed recurring cost | Monthly allocation |
| --- | ---: |
| App/API/Postgres and worker compute, with PDS if offered | $25 |
| Media, bandwidth and off-site backups | $10 |
| Mail, monitoring and security tooling | $8 |
| Billing/payment cost allowance | $6 |
| 45 minutes of support/maintenance at $60/hour | $45 |
| Total direct recurring cost | $94 |
| Contribution before development/acquisition/overhead | $55 |

At two labor hours per month, the same example costs $169 and loses $20. That sensitivity makes repeatable support and narrow scope more important than cheap servers. Allocated costs must be replaced with actual invoices and measured labor. No customer-count or request-capacity claim follows from this budget.

Test a setup fee of $300-$750 and $99-$199/month for a small, bounded organization package. This is an interview and paid-pilot price hypothesis. Storage/mail/team limits must follow measurement. Quote unusual migrations or feature work separately. Avoid low per-account pricing as the main unit when the paid object is the whole organization installation.

Recruit three unrelated paying organizations for an eight-week event-focused pilot. Before inviting them, demonstrate the lifecycle on a disposable branded installation and rehearse full app/database/media/secret recovery. Count onboarding time and all recurring support. Ask customers to renew because they ran useful real work, and record which workflow they value.

Proposed continuation criteria: three customers pay; at least two renew; each completes and repeats its intended workflow; onboarding approaches two operator hours after the first install; direct contribution remains positive after valuing labor; recovery and exit cover both public and private data. These thresholds justify a next experiment, not a market-size conclusion.

### Gaps
- Demand, churn, final pricing, compute allocation, traffic/media usage, support time and recovery objectives remain unmeasured.
- PDS-only operations.md remains useful component research but must not be treated as a budget for the managed Subcult/Patchwork application service.
