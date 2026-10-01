# Managed creator and organization hosting strategy

Research checked September 30, 2026. This revision follows the user's clarified product direction. The product is a hosted creator/organization server that meshes with Subcult OS and Patchwork, on a customer-owned domain, with hosting and operations supplied by us. Prices are experiments. See [product-fit.md](product-fit.md) for source inspection and full-stack cost assumptions; [market.md](market.md) and [operations.md](operations.md) are PDS component research.

## What should we sell first?

### Takeaway
Sell a managed home for an event collective or small cultural organization, with a public presence and a working team/event workflow. PDS hosting supports identity and public records as it matures within that package.

### Cited findings
- Subcult OS already represents a complete first event lifecycle in its README and actual route registrations, including workspace invitations, public event pages, free reservations, check-in and reports. [README](/Users/onnwee/Projects/subcult/subcult-os/README.md); [Routes](/Users/onnwee/Projects/subcult/subcult-os/backend/internal/app/app.go)
- Patchwork represents a complementary mutual-aid application, but current startup still uses fixture services and in-memory organization state. [Pinned API startup](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/index.ts); [Organization service](https://github.com/PatrickFanella/patchwork/blob/7498e47f2471b349e0f3fa2e8415f150b5e28ad9/services/api/src/org-portal-service.ts)

### Inferences
- Lead with "Bring your domain. Launch your own home for your collective, events and community. We run the hosting, updates and backups."
- First customer hypothesis: independent event organizers, artist collectives and small venues with recurring events and a small operating team.
- Initial scope: one organization installation, branding/domain setup, public event/discovery pages, private workspace/team roles, free reservations, door check-in, reports, media/mail allowances and bounded support.
- Keep a repeatable supported package. Selling custom application work to every customer would change the economics into an agency business.
- Offer Patchwork-style requests/offers, resource directories and volunteer coordination only after choosing one buyer-funded journey and replacing its fixture/persistence gaps.

### Gaps
- No paid demand or segment size is established. Interview actual organizational buyers before claiming that creators broadly need the package.
- A general creator website, membership/paywall product and mutual-aid network are distinct offers. The first pilot should establish one repeatable job.

## How should BYO domain and ownership work?

### Takeaway
The customer owns their domain and operational data; we operate the installation. Separate the branded application address, identity handle and PDS endpoint in onboarding and exit terms.

### Cited findings
- Domain handles can be used without changing PDS hosts. [Bluesky domain guide](https://bsky.social/about/blog/4-28-2023-domain-handle-tutorial)
- AT documentation distinguishes data hosting from application hosting and requires distinct app/PDS domain arrangements. [Self-hosting](https://atproto.com/guides/self-hosting); [Production guidance](https://atproto.com/guides/going-to-production)
- Subcult OS assigns private operations to Postgres and intended public cultural records to creator PDS repositories; provisioning and simultaneous Patchwork integration are excluded from its first consolidated release. [Development brief](/Users/onnwee/Projects/subcult/subcult-os/docs/development/development-brief.md)

### Inferences
- Begin with manual provisioning and isolated application/database installations per pilot customer. Choose the PDS endpoint separately after security and identity review; avoid assuming `pds.customer-domain` is an appropriate companion to an OAuth app on that same site.
- Keep customer infrastructure administration in a separate hosting system. Application workspace membership does not grant fleet administration.
- The customer operates their community and authorizes public publication. We maintain infrastructure, upgrades, monitoring, backups, restore and stated support.
- Exit covers database exports, media, domain/DNS handover and relevant identity/PDS migration. Do not describe PDS portability as portability of all private application records.

### Gaps
- Control plane, branding/domain routing, complete application exports, fleet recovery and billing still need concrete design and qualification.
- Current Subcult OS AT OAuth is disabled pending live qualification and public AT writing is not implemented. Sell demonstrated application workflows before making federation promises. [README](/Users/onnwee/Projects/subcult/subcult-os/README.md)

## How should we validate demand and economics?

### Takeaway
Run a paid concierge pilot around one useful organizational workflow. The recurring subscription buys the whole maintained service, so costs must include applications and operator time.

### Cited findings
- Application hosting and data hosting are distinct operations in the AT stack. [Self-hosting](https://atproto.com/guides/self-hosting)
- Subcult OS requires web, API, Postgres, media, configured mail and optional workers. [README](/Users/onnwee/Projects/subcult/subcult-os/README.md)

### Inferences
All prices and thresholds are proposed experiments.

1. Interview 10-15 event collectives, small venues or organizations about their last actual event, team handover, scattered tools and hosting work. Identify who pays and which existing tools this replaces.
2. Show a branded organization demo that runs the event lifecycle. Explain domain ownership, support limits and exports in ordinary language.
3. Test $300-$750 setup and $99-$199/month for one bounded application package. Seek three unrelated paid pilots over eight weeks, then a real renewal decision.
4. Provision manually and measure DNS setup, app deployment, onboarding, support, updates and recovery labor. Count all time, including event-time assistance.
5. Continue when at least two renew, customers repeat their workflow and the package earns positive contribution after direct labor. Narrow or change the offer when customers primarily request unrelated custom development.

Use the full-stack formula in [product-fit.md](product-fit.md). At an illustrative $149/month, $49 of non-labor direct costs and 45 minutes at $60/hour leave $55 before acquisition/development/overhead. Two labor hours lose $20. These are chosen assumptions, not measured operating margins. The earlier $79/month PDS-only hypothesis is superseded for this full product.

Defer self-service provisioning, multiple application packages, multi-region operations, general plugin hosting and an independent full-network relay until customers pay for a narrower offer. Let the first hosted organizations reveal which shared-network capability matters next.

### Gaps
- Founder capacity, acquisition channels, price tolerance, renewals, invoices, media traffic, backups and recovery times remain unmeasured.
- This research supports a product hypothesis and first experiment. It does not establish market traction or certify a production service.
