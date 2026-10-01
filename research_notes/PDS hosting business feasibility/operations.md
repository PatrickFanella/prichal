# Technical and operating feasibility of managed AT Protocol PDS hosting

Research checked September 30, 2026. Scope is a bootstrapped service for creators and organizations using their own domains. Prices below are published rates checked today. Product prices, usage, labor, and account counts are explicit planning assumptions, not observed demand or benchmarks.

## What would the service actually provide?

### Takeaway
Sell managed account hosting, domain onboarding, migrations, backups, and recovery support. A hosted PDS does not itself provide an independent social network, guaranteed Bluesky distribution, private file storage, or organization publishing workflows.

### Cited findings
- A PDS hosts and distributes account data, manages identity, and routes requests to application services. Relays aggregate updates into a firehose. AppViews provide search, discovery, and aggregated social metrics. Clients can depend on particular AppViews and mandatory labelers. [Protocol overview](https://atproto.com/guides/overview)
- The reference configuration uses Bluesky's AppView, report service, and relay by default. It requires invitations and enables rate limits. Its default single-blob upload ceiling is 300 MB. This is a server limit, not a promise that every application accepts that size. [Reference environment configuration](https://raw.githubusercontent.com/bluesky-social/pds/main/sample.env)
- Handles are mutable DNS hostnames linked bidirectionally to permanent DIDs. The preferred individual setup uses a TXT record at `_atproto.<handle>` containing `did=<DID>`. Only one valid DID record should exist. An HTTPS well-known endpoint is an alternative. Handle validation must check the DID's reverse claim. [Handle specification](https://atproto.com/specs/handle)
- The PDS endpoint is a separate setting in the DID document. A migration changes that endpoint and signing credentials while retaining identity. [Account hosting specification](https://atproto.com/specs/account)
- Ordinary repository records are public. Spaces entered alpha on August 20, 2026. The official announcement explicitly forbids production use and warns against moving real accounts to the alpha. Spaces add access control, not encryption. [Spaces alpha announcement](https://atproto.com/blog/atproto-spaces-alpha)
- Labels support application moderation. Ozone provides labeling decisions; Osprey provides event analysis and automated actions. [Moderation guide](https://atproto.com/guides/moderation)

### Inferences
- BYO domain onboarding should normally add the handle TXT record while leaving website, mail, nameservers, and registrar ownership with the customer. A handle such as `@customer.com` can point to an account on a provider-owned PDS endpoint. It does not require pointing the customer's website A record to the provider.
- Distinguish two products in sales copy. A custom domain handle on existing free Bluesky hosting only requires identity configuration. Moving account data to managed hosting provides a separate benefit and carries migration and recovery responsibilities.
- Domain ownership does not give the customer custody of DID rotation keys. Offer an optional customer-held recovery key and explain its secure storage duties separately.
- For organizations, sell several independently hosted accounts and administrative assistance initially. Do not imply that PDS hosting includes editorial approvals, per-employee access roles, shared-account audit trails, analytics, or a social media scheduling product without separate implementation and verification.
- Shared hosting can use provider PDS endpoints and customer handles. A dedicated instance can provide isolation and a stable customer-specific endpoint. A customer-owned endpoint requires additional domain and origin-security design; it should not be casually placed alongside a future OAuth client application.
- Do not promise immunity from Bluesky moderation or uninterrupted visibility. Downstream services retain their own content and distribution policies.

### Gaps
- No reviewed source established demand for managed hosting or willingness to pay for domain onboarding. No paid-product assumption below is a market estimate.
- No verified production release of Spaces was found in this pass. Treat gated community and private-data hosting as research work, not launch scope.

## What operational duties and risks would the provider take on?

### Takeaway
The install is small; the ongoing responsibility is account availability and identity custody. Backups must restore databases, blobs, private preferences, and protected credentials, then pass a real federation check.

### Cited findings
- The official PDS README requires public IPv4, DNS, and inbound ports 80 and 443. Its recommendation is Ubuntu 24.04, 1 GB RAM, one CPU, 20 GB SSD, and 1 to 20 users. This is deployment guidance, not a measured capacity guarantee. Caddy handles TLS. SMTP supports account verification and other email. The README provides health and WebSocket checks, logging, monitoring, and update instructions. It warns that wiping or improperly moving a host can desynchronize its relay stream. [Official PDS repository](https://github.com/bluesky-social/pds)
- The compose file runs PDS, Caddy, and a daily Watchtower updater. It mounts `/pds` and exposes the Docker socket to Watchtower. [Official compose file](https://raw.githubusercontent.com/bluesky-social/pds/main/compose.yaml)
- Official production guidance calls for separate registered domains for a PDS and an application because untrusted blobs can create credential-theft risks. Changing an active PDS hostname requires individual account migrations. SQLite uses per-user repository databases and shared PDS databases. The guide recommends Litestream backups and recovery that reapplies operations from the PDS firehose. It recommends protecting the provider PLC rotation key in a KMS or HSM. [Going to production](https://atproto.com/guides/going-to-production)
- Migration includes repository CAR data, separate blob transfers, private preferences, identity changes, and account activation. Large repository imports may fail. The reference PDS limits blob uploads to 1,000 per IP per day; trusted migration tooling may bypass limits. Some preferences or chat state live outside the PDS. Services with stale identity caches may fail for up to 24 hours after migration. [Account migration guide](https://atproto.com/guides/account-migration)
- By default the PDS holds the sole PLC rotation key. A customer can register a higher-priority key to recover identity independently. Recovery still requires repository, blob, and preference backups. A stolen private rotation key can let an attacker take over identity. [Account recovery guide](https://atproto.com/guides/account-recovery)
- Blobs are authoritative on the PDS and often distributed through application CDNs. They become public when referenced by records. Operators may impose storage quotas, size limits, and content policies, but limits can affect application compatibility. Blob removal follows record and account lifecycle changes. [Blob specification](https://atproto.com/specs/blob)
- Relays and AppViews independently decide what they redistribute. Hosting states include suspension and takedown, and downstream services should respect upstream inactivity. Rate limits protect against resource exhaustion. [Account lifecycle specification](https://atproto.com/specs/account)

### Inferences
- Initial production scope should include invite-only onboarding, bounded storage, migration intake checks, email delivery monitoring, disk alerts, security patch review, external health checks, and a documented exit procedure. Do not offer an availability SLA before measuring operations and testing recovery.
- Test restore to an isolated host using disposable accounts. Verify authentication, repository signatures and revisions, all referenced blobs, preference recovery, and downstream indexing of a newly written record. A successful filesystem copy is insufficient.
- Keep off-host encrypted backups and protect backup access separately from production administration. Preserve keys and secrets with recovery controls. A customer-facing export is useful but cannot replace a provider disaster recovery plan.
- Watchtower's default automatic updates merit deliberate review before a paid service. A staging account, controlled rollout, rollback procedure, and post-update federation check reduce the chance that an overnight upgrade breaks every customer.
- Shared PDS hosting reduces per-account infrastructure cost but couples customers to the same outage, operator credentials, resource pool, and hostname. Multiple independent PDS instances provide a practical initial partition. Choose thresholds from load tests and live measurements, not the account count alone.
- Support includes DNS propagation, expired domains, email delivery, forgotten passwords, OAuth/client login issues, migration failures, missing media, and downstream indexing. Quote onboarding separately from recurring support so complex migrations do not erase several months of subscription revenue.
- Hosting public user uploads requires an abuse contact and a process for reports, evidence, suspensions, appeals, and lawful requests. Application labelers do not eliminate the provider's own storage and account actions. Jurisdiction-specific legal duties need a separate qualified review before launch.
- Avoid selling unlimited data. A customer's use across multiple apps can exceed a Bluesky-only storage estimate, and migrations plus backup retention create extra disk and network use. Measure actual origin egress rather than assuming every viewer fetches media from the PDS.

### Gaps
- No load benchmark, real account workload distribution, restore drill, tested failover, or audited production service was performed. No supported accounts-per-server claim is justified.
- This pass did not verify native organization roles or end-to-end customer quotas in a commercial control panel. Treat provisioning, billing, quota enforcement, and support tooling as additional work.
- No insurance, legal compliance cost, incident frequency, support-ticket rate, or cloud-provider approval for this particular business was established.

## What do illustrative creator and organization economics look like?

### Takeaway
At small scale, founder support and maintenance time can cost several times the cloud bill. A $15 creator account can work with bounded support; dedicated organization plans need higher prices and separate setup fees.

### Cited findings
- DigitalOcean regular Basic Droplets list $12 monthly for 2 GiB RAM, one vCPU, 50 GiB disk, and 2,000 GiB transfer; $24 buys 4 GiB RAM, two vCPUs, 80 GiB disk, and 4,000 GiB transfer. Daily backups cost 30% of the VM rate. [Droplet pricing](https://www.digitalocean.com/pricing/droplets)
- DigitalOcean block storage costs $0.10 per GiB monthly. [Volume pricing](https://docs.digitalocean.com/products/volumes/details/pricing/)
- DigitalOcean pools Droplet transfer allowances by team. Inbound transfer is free; outbound overage is $0.01 per GiB. Public backup uploads consume outbound transfer. [Bandwidth billing](https://docs.digitalocean.com/platform/billing/bandwidth/)
- Backblaze B2 currently lists $6.95 per TB monthly, a first-10-GB free allowance, and free egress up to three times average stored data. Further nonpartner egress costs $0.01 per GB. [B2 pricing](https://www.backblaze.com/cloud-storage/pricing)
- Stripe US domestic card processing is 2.9% plus $0.30 per payment. Pay-as-you-go Billing adds 0.7% of Billing volume. International cards, currency conversion, and disputes can cost extra. [Stripe pricing](https://stripe.com/pricing)

### Inferences
All figures in the model below are USD per month unless stated otherwise. Commercial terms are hypotheses for customer interviews, not recommended final list prices.

| Proposed option | Assumed price | Proposed boundary |
|---|---:|---|
| Creator on shared infrastructure | $15 | One account, BYO handle, 2 GiB storage ceiling, bounded asynchronous support |
| Organization on shared infrastructure | $49 | Up to five independent accounts, 10 GiB total ceiling, account administration help; no invented publishing roles |
| Dedicated organization PDS | $99 | Stable dedicated endpoint, up to five accounts, 20 GiB ceiling, bounded asynchronous support |
| Migration and domain setup | $99 standard or $199 complex, one time | Quote after checking repository size, blob count, old-host access, email, and domain controls |

The monthly illustration assumes 20 paying creator accounts and five dedicated organization instances. It deliberately omits shared organization accounts to keep the arithmetic easy to audit. Five accounts on a dedicated instance is a proposed plan limit, not a capacity benchmark.

| Item | Calculation | Monthly amount |
|---|---|---:|
| Revenue | 20 × $15 plus 5 × $99 | $795.00 |
| Shared VM | One $24 VM, conditional on future workload testing | $24.00 |
| Dedicated VMs | Five × $12, conditional on future workload testing | $60.00 |
| Daily VM backups | 30% × $84 | $25.20 |
| Added shared volume | 50 GiB × $0.10 | $5.00 |
| Independent backup storage | Assumed 200 GB retained, × $0.00695 per GB, ignoring the small free allowance | $1.39 |
| Traffic overage | Assumed below pooled included allowance, including backup uploads | $0.00 |
| Monitoring, mail, domain, billing app, miscellaneous service budget | Planning allowance, not vendor quote | $60.00 |
| Card processing and Billing | $795 × 3.6% plus 25 × $0.30 | $36.12 |
| Routine account/customer support | 20 × 0.1 hour plus 5 × 0.5 hour | 4.5 hours |
| Maintenance, updates, backup checks, abuse triage | Assumed founder time | 4 hours |
| Total valued labor | 8.5 hours × assumed $50 hourly cost | $425.00 |
| Surplus after modeled operating costs and labor | $795 minus $211.71 nonlabor costs minus $425 labor | $158.29 |

This is about 20% of revenue after valued routine labor. Cash before paying the founder is $583.29, which should not be mistaken for owner profit. The model excludes acquisition, product development, legal work, taxes, insurance, paid on-call coverage, refunds, incidents, and financing. It therefore overstates business profit.

The retained 200 GB backup volume is an assumption about actual use and retention, not a full daily copy of every purchased quota. If backups retain 30 complete copies, storage grows much faster. Deduplication, incremental methods, retention policy, deleted-content handling, and actual churn must be tested. VM snapshots also do not establish database consistency or include a separately attached volume automatically.

Sensitivity matters more than a single margin. An extra 15 support minutes for each of the 25 customers costs $312.50 at the assumed wage and turns the $158.29 surplus into a $154.21 loss. One extra 1,000 GiB of outbound transfer costs $10, while ten unexpected founder hours cost $500. The dedicated plan has $15.60 in VM plus daily backup cost before support, fees, storage, and overhead. A one-hour support case costs another assumed $50.

A $99 setup payment leaves $95.829 after domestic card and Billing fees if Billing applies, or $95.829 calculated as $99 minus $3.564 minus $0.30. At two hours of $50 labor, setup already loses $4.171 before other costs. A $199 complex migration yields $191.536 after those fees, allowing roughly 3.83 hours of labor before other costs. Treat these as budgets; migration complexity may justify a different quote. If a one-time payment does not incur the Billing fee, margins improve slightly.

Annual collection reduces the fixed $0.30 payment component from $3.60 yearly to $0.30 yearly per customer. It does not remove support or create service capacity. Do not depend on annual cash to hide an unprofitable recurring service.

### Gaps
- Storage and labor assumptions need a pilot. Measure creator and organization support separately, migration time, active data growth, backup churn, origin transfer, and minutes spent on updates and abuse.
- The model is a small single-region operation with backup restoration, not highly available hosting. Larger availability promises require additional infrastructure and staffed response costs.
- Vendor rates can change and GiB differs from decimal GB. Obtain an actual region-specific estimate before provisioning; this is a feasibility worksheet, not a cloud quote.
