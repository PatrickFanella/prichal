# Launch independent community homes through hosting

*Decision report, September 30, 2026. Based on the research notes and repository source inspection completed for this question.*

**The strongest business hypothesis is a managed home for creators and organizations on their own domain, built around Subcult OS and, later, qualified Patchwork capabilities.** The customer brings their domain and runs their community; we supply the applications, hosting, updates, backups and support. PDS hosting belongs inside this product as infrastructure for identity and public records. The broader vision includes events, collective operations, mutual aid and member coordination. An events-first launch is a proposed way to test that vision with an existing workflow, not a permanent restriction chosen by the user. Start with a paid, manually provisioned pilot before building self-service, and test whether customers renew after doing real work. Current evidence supports this experiment, but does not establish demand, final pricing or production readiness.

## Customers buy a working organizational home

The proposed pitch is **"Bring your domain. Launch your own home for your collective, events and community. We run the hosting, updates and backups."** A customer should recognize a place where their team can operate and their audience can participate. Selling the product as "PDS as a service" would obscure most of that value and invite comparison with inexpensive account hosting.

The first buyer hypothesis is an independent event organizer, artist collective, small venue or cultural organization with recurring activity and a small team. Subcult OS already documents workspace ownership and invitations, published events, free reservations, door check-in and closeout reports, and its API registers corresponding handlers. That provides a concrete launch sequence to demonstrate and qualify. A solo creator who wants only a website is a weaker initial fit because the package's organizational workflow adds less value for them. These are source-informed hypotheses, not interview findings. ([Subcult OS README](/Users/onnwee/Projects/subcult/subcult-os/README.md), [API routes](/Users/onnwee/Projects/subcult/subcult-os/backend/internal/app/app.go))

The full vision should remain visible. Patchwork supplies a complementary direction for requests and offers, volunteer coordination and resource directories. Together, the applications suggest a hosted home where a collective can publish activity, run its internal work and organize community participation. The first supported package should make one of those journeys dependable, then add another when a paying customer identifies the need. A different pilot sequence would be reasonable if interviews establish that mutual aid or member coordination is the urgent problem.

A proposed launch journey makes the offer tangible:

| Customer stage | Customer experience | Provider work |
| --- | --- | --- |
| Choose a home | Select the supported package, name an owner and bring a domain | Explain the application address, identity handle and any PDS endpoint |
| Launch | Open a branded installation and invite the team | Configure DNS, TLS, applications, database, media, mail and backups |
| Complete useful work | Publish an event, accept free reservations, check people in and review the report | Resolve onboarding issues and monitor the services |
| Keep operating | Repeat the workflow and manage community decisions | Maintain software, test recovery and provide stated support hours |
| Expand or leave | Add a qualified module, or take domain and data elsewhere | Supply application exports, media and relevant PDS migration assistance |

For the initial package, include one organization installation, public pages, its private workspace, team invitations, free reservations and check-in, reports, media and mail allowances, and bounded support. Paid tickets, subscriptions, marketing campaigns, arbitrary plugins and custom development need separate scope. Standardization matters: recurring bespoke feature work would produce agency economics rather than a repeatable hosting business.

## Existing applications provide a starting point, with hosting work still ahead

The source review pinned Subcult OS at `d40221f00a9bb6790240e310cde1b3b2c6497b9d` and Patchwork's public mirror at `7498e47f2471b349e0f3fa2e8415f150b5e28ad9`. It establishes what source and documents contain. It does not establish a successful commercial deployment or parity with Patchwork's private source.

| Component | Verified source evidence | Work before selling the corresponding capability |
| --- | --- | --- |
| Subcult OS organizational workflow | README and routes represent ownership, invitations, events, reservations, check-in and reports | Run the complete journey on a branded installation; qualify configuration, exports and recovery |
| Subcult OS AT connection | README describes identity-only OAuth, disabled pending live qualification; public AT writing is unimplemented | Qualify identity integration and implement a demonstrated publication/discovery journey |
| Patchwork community services | Web/API, indexing and moderation structure exists; API startup still initializes fixture services | Replace relevant fixtures and qualify one persistent customer journey |
| Patchwork organizations and tenancy | Organization portal and tenant model use in-memory state | Implement persistence, real boundaries and deployment behavior before promising these functions |
| Commercial hosting | Applications supply useful product building blocks | Add customer/domain inventory, repeatable deployment, secrets, updates, billing, metering, backups and support operations |

The Subcult OS development brief separates private contacts, staffing, finances, attendance and consent from public cultural records. It also excludes PDS provisioning and simultaneous Patchwork integration from its first consolidated release. The hosting product therefore needs its own delivery plan around these applications. It should preserve their data boundaries and avoid assuming that application workspace permissions authorize infrastructure administration. ([Development brief](/Users/onnwee/Projects/subcult/subcult-os/docs/development/development-brief.md), [README](/Users/onnwee/Projects/subcult/subcult-os/README.md))

Patchwork's gaps affect what can be promised at launch. Its API selects PostgreSQL for queries when configured while initializing fixture authentication, chat, volunteer, settings and verification services separately. Its organization service stores organizational records in memory, and its tenant service explicitly models boundaries and failover in memory. The default Compose configuration also omits the indexer database URL that selects persistent operation. These findings justify qualification and implementation work, not abandoning the mutual-aid direction. ([Pinned API startup](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/index.ts), [organization service](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/org-portal-service.ts), [tenant service](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/tenant-service.ts), [Compose](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/docker-compose.yml), [indexer](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/indexer/src/index.ts))

For the pilot, use an isolated application installation and database per organization on a shared managed fleet. Describe that arrangement accurately rather than promising a dedicated physical server. Manual provisioning is acceptable while the package is changing. A shared multi-tenant application, automatic provisioning and multiple regional deployments can follow measured costs and customer needs.

## PDS connects identity and public data to the wider network

AT Protocol separates account data hosting from application hosting. A PDS hosts account records and identity functions; relays aggregate updates, and AppViews supply application-specific search and discovery. Installing a PDS does not supply the Subcult OS or Patchwork user experience, nor does it guarantee distribution by downstream applications. ([AT Protocol overview](https://atproto.com/guides/overview), [self-hosting guide](https://atproto.com/guides/self-hosting))

**BYO domain should mean customer ownership with managed setup.** Keep the branded application address, the identity handle and the PDS endpoint distinct in onboarding. A customer can use a domain handle on existing Bluesky hosting without buying a PDS, so domain verification alone is a weak reason to subscribe. AT production guidance also calls for separate registered domains for PDS and application hosting because of origin and blob security concerns. Do not assume an application and its PDS should sit on neighboring customer subdomains. ([Bluesky domain guide](https://bsky.social/about/blog/4-28-2023-domain-handle-tutorial), [AT production guidance](https://atproto.com/guides/going-to-production))

Keep private organizational work in the application database. Ordinary AT repository records are public; the Spaces alpha announcement forbids production use and describes access control without encryption. Public cultural publishing should become a supported feature only after its complete publication and discovery journey works. Customers should control which records enter that public network. ([Spaces alpha announcement](https://atproto.com/blog/atproto-spaces-alpha), [Subcult OS development brief](/Users/onnwee/Projects/subcult/subcult-os/docs/development/development-brief.md))

Ownership also needs a practical exit. Application records and media require their own exports; a PDS repository export does not include all private organizational operations. PDS migration itself involves repositories, blobs, preferences and identity changes. Customer-held recovery keys can support independent identity recovery, but they do not replace data backups. The product's portability promise must cover the whole installation. ([Account migration guide](https://atproto.com/guides/account-migration), [account recovery guide](https://atproto.com/guides/account-recovery))

## Compete on organizational outcomes and price the whole service

Generic PDS hosting already has cheap and free alternatives. Periwinkle advertises a $4 monthly Basic plan and $14 Pro plan; its $30 Team plan is marked as launching soon. eny.space advertises €9 Personal and €24 Community plans, with an organization plan still planned. selfhosted.social and Eurosky offer free hosting. These are published offers checked for this research, not evidence of paid demand or operational quality. ([Periwinkle plans](https://periwinkle.social/plans), [eny.space](https://eny.space/), [selfhosted.social FAQ](https://selfhosted.social/info), [Eurosky accounts](https://eurosky.tech/accounts/))

The relevant alternatives for this broader product also include customers keeping their current collection of tools, self-hosting these applications, or paying someone to build a custom site. The hypothesis to test is that a coherent organizational workflow with accountable operation earns a premium. Interviews should identify what it replaces, who controls the budget and whether the customer values the applications enough to pay repeatedly. The notes establish no reliable market size, conversion rate or retention evidence.

Test **$300-$750 setup and $99-$199 per month** for one bounded organization package. These are interview and pilot assumptions, not validated prices. The paid unit should be the maintained organization installation, with explicit usage allowances. Quote unusual migrations and feature work separately.

The budget must include web/API/database compute, media and bandwidth, mail, background workers, backups, monitoring, payment fees, PDS costs when included, and direct maintenance and support labor. The earlier PDS-only budgets do not price this product. This full-service example deliberately uses chosen allocations rather than provider quotes:

| Illustrative monthly allocation at $149 subscription | Amount |
| --- | ---: |
| Application/API/Postgres and workers, with PDS if offered | $25 |
| Media, bandwidth and off-site backups | $10 |
| Mail, monitoring and security tooling | $8 |
| Billing and payment allowance | $6 |
| 45 minutes of direct support/maintenance at $60/hour | $45 |
| Total direct recurring cost | $94 |
| Contribution before development, acquisition and overhead | **$55** |

At two direct labor hours a month, the same assumptions produce $169 in costs and a $20 loss. Support scope and repeatable operations therefore matter more than choosing the cheapest VM. Replace every allocation with invoices and measured time during the pilot. These figures imply no server capacity or final operating margin.

## A paid pilot should test repeat use and manageable operation

Interview 10-15 prospective organizational buyers about their last actual event or community project, team handover, scattered tools and infrastructure work. Then demonstrate a branded installation that completes the supported journey. Recruit three unrelated paying organizations for an eight-week pilot and ask for a real renewal decision. Events are the recommended first demonstration because Subcult OS has source evidence for that lifecycle; the interviews can justify a different first package.

Before onboarding, run the complete workflow on a disposable installation and restore its application, database, media and secrets. If PDS hosting is included, recovery must also verify identity, repository data, blobs and a new record's downstream indexing. Count DNS setup, deployment, onboarding, event-time assistance, updates and recovery work as operator time. Controlled updates and tested restoration belong in the service sold to customers. ([AT production guidance](https://atproto.com/guides/going-to-production), [official PDS repository](https://github.com/bluesky-social/pds))

Proposed continuation criteria are three paid pilots, at least two renewals, repeated completion of the intended workflow, onboarding approaching two operator hours after the first installation, and positive contribution after valuing direct labor. Recovery and exit must cover private application data as well as public identity data. These are decision thresholds for the next experiment, not proof of a large market.

The executive recommendation is to advance the managed organizational-home concept through this pilot. Use Subcult OS as the initial application base and preserve Patchwork as an explicit community capability to develop against paid demand. Defer broad self-service, arbitrary application packages and an independent full-network relay while learning what customers actually use. If they mostly request unrelated custom development, change the package or recognize that the business has become a services agency.

## Conclusion

The next investment should create a reviewable customer offer and a recoverable demo, then secure payment for useful organizational work. That sequence tests both sides of the business: whether customers want this home and whether a small operator can maintain it at the proposed price.

If renewal and labor measurements support expansion, the next module should follow the customers' repeated need. Patchwork mutual aid or coordination can then become a funded part of the hosted product, while AT publication connects suitable public activity across applications. This preserves the independent creator and organization vision without making the first launch depend on every integration at once.
