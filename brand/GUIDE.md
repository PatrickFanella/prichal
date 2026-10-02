# Prichal brand guide

Draft, 2026-10-01. The name, palette, type and messaging are chosen. The mark is a first draft and has not been reviewed by the owner at production sizes.

## Name

**Prichal** is the product name for this repository's managed AT Protocol PDS hosting. The repository, Go module and service identifiers stay `pds-ass` until a separate rename.

Причал is Russian for a pier or berth. It is also the name of the ISS docking node (2021): a sphere with six ports where Soyuz and Progress ships attach. Accounts dock here; we keep the station running.

The pronunciation is the Russian one, pree-CHAHL. Brand copy does not explain or correct it. Readers may say it however they like. If the meaning comes up, one quiet line is enough: *Russian for "pier."*

Write it as **Prichal** in prose and **PRICHAL** only in the wordmark and display headings. The Cyrillic **ПРИЧАЛ** is a secondary label, used small, never in place of the Latin name.

## Lines

| Use | Line |
|---|---|
| Tagline | **Your server. We keep it running.** |
| Call to action | **Dock your account. We handle the rest.** |
| Plain descriptor | Account hosting for the open social web |
| Technical descriptor | Managed AT Protocol PDS hosting (developer docs and technical audiences only) |

Lead with what customers get: their own server without the upkeep. Do not market leaving. Portability is a property of the protocol that every host offers. State it as reassurance where people look for it (FAQ, pricing, onboarding, policy) and never as a headline.

## Plain language

Most customers do not know what a PDS or AT Protocol is and do not need to.

| Avoid | Say |
|---|---|
| PDS | your account's home server; your server |
| AT Protocol, atproto | the open network Bluesky runs on; the open social web |
| Account migration | move your account |
| Handle on a domain | your name as your username, e.g. `@yourband.com` |
| DID, repository, records, PLC | do not mention |

## Audiences and pitches

Ordered by expected fit. The first comes from the product-fit research in `research_notes/PDS hosting business feasibility/product-fit.md`; the others are untested positioning.

**1. Event organizers, venues and collectives (Subcult OS).** Pitch the event workflow: pages, reservations, check-in, closeout. The server is bundled underneath. "Your events and releases, published from a server you own" waits until Subcult OS publishes public records to a creator server; its first release excludes that.

**2. Organizations bringing their members (Patchwork).** *"Give your members a home on the network, under your name. We keep it running."*
- Usernames that show membership: `@maria.eastsidetenants.org` says she belongs, because only the organization can issue those names.
- The organization decides who gets an account.
- Members' accounts are held by an organization they chose, not an ad-funded company.
- Members sign in to Patchwork with those accounts from day one.

Not yet available. The roadmap covers one owner's first account per server and organization management roles (#30). Inviting members to create their own accounts on an organization's server needs a new feature: invitations, limits, removal and what happens to a member's account when they leave. Do not market this pitch until that exists.

**3. Musicians and creators with a following.** *"Your name, your followers, your posts, on a server that's yours. Post from Bluesky like always. We keep it running."*
- The audience follows the creator, not an account a platform company holds.
- `@yourname.com` as their username, set up with everything else. Domain usernames also work on Bluesky's own hosting, so this is a convenience, not an exclusive feature.
- One sign-in across apps on the network, Bluesky and Patchwork included.

**4. Bluesky users.** *"Keep your Bluesky account on a server of your own. Same app, same followers."* The weakest fit: most casual users have no problem this solves. Expect people who value independence or community-run infrastructure.

## Claims to avoid

- **Private.** Posts and records on the network are public. An organization's own server is not a private space. Private data (contacts, case notes, needs, finances) belongs in a separate application database. Say this plainly to mutual aid and tenant organizations.
- **Can't be banned, censorship-proof.** Each app decides what it shows, wherever the account lives.
- **Leave anytime.** Hosting exits have retention windows and recovery steps.
- **Anything not yet verified.** Public hosting is not available. Check `docs/progress/local-m1-alpha.md` before making a capability claim.

## Voice

Plain, specific and calm. State what happens, when, and what the customer keeps. No hype, exclamation marks or space-race bravado. Docking language ("dock", "home port") is for headings and calls to action; it never replaces the precise term where a decision or consequence is involved. There is no figurative language for leaving.

Product UI, emails, receipts and policy text use the terms in `CONTEXT.md`, such as *hosting exit*, *retention window* and *payment cancellation*.

"Attik" is a proposed name for the export and backup area. It is not adopted.

## Mark

A docking node: a sphere with its equator drawn in perspective and four collars with flanges. The red flange is the occupied port: the customer's account, docked.

- Clear space: one flange height (about 7% of the mark width) on every side.
- Minimum size: 32 px on screen. Below 24 px use the mono mark.
- The red flange is always on the right. Do not rotate, recolor individual collars, or add a centre dot or inner ring. With the cross of ports, a centred ring reads as a gun sight.
- Mono variants turn the flange to ink.

Files: `logos/mark*.svg` (editable), `logos/*-outlined.svg` (text converted to paths), PNG exports.

## Color

| Token | Hex | Role |
|---|---|---|
| `background` | `#f4f1e9` | Paper. Matches the current web app background. |
| `text` | `#1c2230` | Ink. Text, mark, dark fields. |
| `accent` | `#b23a26` | Signal red. Occupied port, rules, emphasis. |
| `secondary` | `#5d6675` | Steel. Labels and supporting text. |
| `rule` | `#c9c4b6` | Hairlines and tick scales. Decorative only. |
| `dark_accent` | `#e8674b` | Signal red on ink fields. |

Measured contrast for ordinary text is in `tokens.json`; every text pairing is at least 4.5:1, and `build.py` refuses to build otherwise. `rule` is below 4.5:1 and must not carry text. Use red for emphasis, not body copy.

## Type

- **Oswald Bold**, uppercase, 0.06em tracking: wordmark and display headings.
- **Public Sans**: body copy and taglines, sentence case.
- **JetBrains Mono**, uppercase, wide tracking: labels, port strips, metadata.

All three are OFL and bundled in `fonts/` with their licenses. Oswald and JetBrains Mono include Cyrillic.

## Layout motif

Engineering-drawing details: a tick scale on the left edge, a hairline above the footer, a port strip (`PORT 01 · 02 · 03 · 04 · 05 · 06`) and mono metadata. Keep them light; the mark carries the identity. Story content stays between y=250 and y=1670. Video layouts use a 96 px outer margin.

## Building

```sh
python3 brand/build.py --render --outline
```

Needs Python 3, `rsvg-convert` for PNGs and Inkscape for outlined SVGs. The builder regenerates `logos/`, `social/`, `video/`, tokens and the manifest from `brand.json`. Edit `brand.json` or `build.py`, not the exports. The output follows the shared Subcult starter-template layout so the pack can move into the branding library.

## Before publishing

- Replace `prichal.example`. No domain has been registered. On 2026-10-01 `prichal.com`, `.net` and `.app` were registered by others; `.io` returned no WHOIS record; `.dev`, `.host`, `.social` and `.cloud` had no DNS records. Confirm with a registrar.
- No trademark search has been done.
- Layout copy is placeholder until the claims behind it are verified.
- Write alt text for finished images; starters are in `alt-text.md`.
