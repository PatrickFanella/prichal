# Local organization collaboration contract

A management identity is an exact issuer/subject pair. An invitation names an already verified management identity reference, not an email or display name. Only the organization owner issues invitations and changes membership. Acceptance requires that same identity, is single-use and expires after 24 hours. Revocation takes effect on subsequent requests.

| Capability | Owner | Administrator | Viewer |
| --- | --- | --- | --- |
| Shared organization name and safe server status | Yes | Yes | Yes |
| Rename organization | Yes | Yes | No |
| Invite, change roles, revoke members | Yes | No | No |
| Empty-organization ownership handover | Yes, with confirmation | Accept ownership as invited target | No |
| Existing deployment, domain, billing, support, export and exit controls | Existing owner authority | No delegated mutation in this slice | No |
| Native signing, publishing, password or private account content | Separate native account authority | Separate native account authority | Separate native account authority |

Shared status is an explicit small projection. It excludes enrollment invitations, export custody, raw errors, native identity values, credentials and billing content. Administrator deployment/domain delegation remains later work; membership does not silently widen existing owner APIs or CLI credentials.

Handover requires a separate expiring offer, explicit acceptance by the target administrator and an organization with no deployment or billing binding. The organization always has one nonnull owner. Accepted membership and all changes are audited atomically. Pending invitations from the former owner are invalidated; the former owner receives no implicit membership. Populated hosting handover requires a separate custody/billing plan and remains unavailable.

This rehearsal is disabled by default. Production startup rejects enabling it. Its tables preserve intent and audit through management recovery. No outbound invitation email, host operation, AT identity merge or native account password sharing is introduced.
