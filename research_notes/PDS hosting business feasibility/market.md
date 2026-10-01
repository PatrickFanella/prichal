# Managed AT Protocol PDS hosting competitors

Research checked September 30, 2026. Prices below are public offers in the vendor's stated currency. They do not establish sales, willingness to pay, or operational performance. No signup, purchase, migration, or recovery test was performed.

## Which managed alternatives exist, and what do they charge?

### Takeaway

Paid PDS hosting is already offered at low consumer prices. A creator plan must compete with Periwinkle's advertised $4 monthly entry tier and free community hosts. Organization hosting has direct competition, although several advertised team products remain in beta or planned.

### Cited findings

| Provider | Published offer | Evidence and limits |
| --- | --- | --- |
| eny.space | Personal €9/month, 5 GB, one DID; Community €24/month, 20 GB, multiple accounts, invite codes; managed backups and updates in both | Site marked beta. Organization plan with dedicated node, custom domain and SLA is coming soon. Personal plan lists a provider subdomain. [Official site](https://eny.space/) |
| selfhost.tools | Advertised $15.99/month equivalent of yearly billing; private instance provisioned within 24 hours; updates, backups, security monitoring and support | Monthly billing is mentioned without a clear separate price in retrieved page. Storage, account limits, migration terms, custom domain support and SLA are not established. Listed testimonials concern other apps, not PDS customers. [Official PDS offer](https://selfhost.tools/bluesky-pds/) |
| Periwinkle | Free 500 MB/two subdomain handles; Basic $4/month, five handles/5 GB; Pro $14/month, 20 handles/25 GB; Team $30/month, 50 handles/100 GB | Basic advertises seven-day real-time backups, EU/US choice and email support. Pro adds 14-day retention, point-in-time recovery and 72-hour email SLA. Team is marked launching soon and advertises dedicated PDS, team controls/SSO and 24-hour priority email SLA. Enterprise advertises custom capacity and 99.99% SLA with credits, with no public price. [Official plans](https://periwinkle.social/plans) |
| Protobase | Private beta and waitlist; announced free evaluation tier, per-server subscriptions, account-count tiers and storage overages | Final prices are explicitly not set. Dashboard has illustrative metrics alongside fictional brand names, so its account counts do not prove traction. [Official site](https://protobase.at/) |
| selfhosted.social | Free, supported by tips/sponsors; open signup without invite code | Provider reports Atlanta DigitalOcean VPS, nightly server backups, account backups through PDS MOOver and direct administrator support. No commercial SLA is established. [Official FAQ](https://selfhosted.social/info) |
| Eurosky | Free account hosting, including a migration path | Dutch nonprofit offers EU hosting through EU-headquartered providers. Public service is a credible substitute for sovereignty-motivated personal customers. [Official accounts page](https://eurosky.tech/accounts/) |
| Blacksky | Open account creation and community infrastructure; no paid hosting tariff established in this review | Some domains are restricted to named communities. Its documentation describes open alternatives and existing-domain handles, rather than a priced managed hosting package for outside organizations. [Account creation](https://docs.blacksky.community/info-about-blacksky-the-community/create-an-account.md), [handle documentation](https://docs.blacksky.community/using-blacksky/account-management/change-my-handle.md) |

### Inferences

- A $9 to $16 generic personal-hosting offer has cheaper published competition. Support-heavy accounts may require more revenue, but these prices give no evidence that customers will accept a premium.
- Selling dependable organization operations could support higher pricing than consumer storage. Published community/team offers already compete in that segment, so distinct service outcomes must be tested with buyers.
- BYO domain is useful onboarding work, but other hosts already support it. A paid package needs value beyond domain verification.

### Gaps

- No independently verified paid customer counts, conversion rates, churn, customer acquisition costs or revenue were found for these commercial providers.
- No tax-inclusive comparison or completed checkout was available. Do not treat selfhost.tools' yearly-equivalent amount as a confirmed month-to-month price.
- Blacksky's current price, storage allowance and any organization-hosting contract were not verified. Its docs were fetched directly with curl after the web fetcher rejected markdown URLs.

## Which claims show maturity, and which organization tools are still planned?

### Takeaway

There is stronger evidence of functioning free account hosting than of a mature commercial market. Existing paid offers demonstrate competitive intent. They do not demonstrate a large customer base or reliable delegated organization administration.

### Cited findings

- Selfhosted.social's public homepage reports 1,371 hosted accounts, 130 active today and 230 this week in the retrieved snapshot. These are operator-published usage figures, not audited counts or paid users. [Homepage](https://selfhosted.social/)
- Eurosky publishes an operator-controlled service-status page, monitored since March 19, 2026 for PDS and April 15 for Portal. The retrieved 90-day window shows 99.975% PDS uptime and 99.984% Portal uptime. This is monitoring evidence, not a contractual guarantee or independent audit. [Status page](https://status.eurosky.tech/)
- Periwinkle's homepage still asks for early-access signup and says it will email when the user's turn arrives. Public plan prices therefore do not establish unrestricted immediate provisioning. [Homepage](https://periwinkle.social/)
- Periwinkle's roadmap says it is building multi-handle organization support, centralized management and onboarding. This agrees with Team's launching-soon label. Current shared-account delegation, role permissions and SSO were not operationally verified. [Roadmap](https://periwinkle.social/docs/roadmap), [plans](https://periwinkle.social/plans)
- Periwinkle documents BYO handles at a domain or subdomain, supplies a DNS record during signup and directs users to connect apps using their hosting-provider URL. This is specific implementation documentation beyond a generic ownership slogan. [Getting started](https://periwinkle.social/docs/getting-started)
- Protobase advertises isolated multi-PDS management, traffic/error charts and audit history in private beta. No permission model, delegated publishing or live customer evidence was established. [Official site](https://protobase.at/)
- eny.space advertises administration and multi-account invitations; its Organization offer remains planned. The retrieved FAQ lists questions about migration and PLC rotation-key control without exposing their answers. These must be checked before comparing custody and exit guarantees. [Official site](https://eny.space/)

### Inferences

- A bootstrapped creator/organization service can test hands-on migration, recovery, DNS onboarding and response commitments before building a broad platform.
- Hosting administration and publishing delegation are separate jobs. A dashboard that invites PDS accounts does not prove that an organization can safely let staff publish through shared handles with revocable roles.
- Team roles, member onboarding/offboarding and recoverable identity custody are plausible needs, but current evidence only shows competitors discussing or promising related tools. Buyer interviews must establish which work is painful and worth paying for.

### Gaps

- No restore drill, export/reimport test, identity-key custody inspection or end-to-end staff delegation test was performed on any provider.
- Public dashboard graphics, testimonials about other hosted apps and labels such as most popular should not be counted as PDS customer evidence.
- Current capability of Periwinkle team roles and eny.space organization tooling needs an actual demo or written vendor confirmation.

## What can the evidence support about unmet demand?

### Takeaway

The sources establish supply, free substitutes and some self-reported non-Bluesky usage. They do not establish the size of a paying creator/organization market. The next useful evidence is a paid pilot for a defined operational problem.

### Cited findings

- Free hosting already includes migration and backups at selfhosted.social, so those words alone do not distinguish a paid service. [FAQ](https://selfhosted.social/info)
- Eurosky combines free account migration with nonprofit European-hosting positioning. Sovereignty messaging alone has a free competitor. [Accounts page](https://eurosky.tech/accounts/)
- Periwinkle advertises BYO domains and paid backups, while organization support remains under development. A proposed domain-aware team service would compete with its stated direction. [Getting started](https://periwinkle.social/docs/getting-started), [roadmap](https://periwinkle.social/docs/roadmap)

### Inferences

- Test a managed service for creators with valuable existing accounts and organizations that need accountable migration/recovery support. Ask for payment to complete the task, rather than relying on positive reactions to decentralization.
- Candidate pilot package: BYO-domain verification, account migration with explicit data checks, documented custody/exit process, scheduled restore drill, stated support hours and member-account administration. Add delegated publishing only if interviews justify a separate product and its implementation constraints are understood.
- Do not estimate this market by multiplying Bluesky registrations by a subscription price. Registered users are not independent-hosting customers, and free-host usage is not willingness to pay.

### Gaps

- Creator/organization interview evidence, signed pilots and comparable quotes are missing.
- No reliable total addressable market or willingness-to-pay estimate follows from the provider pages.
