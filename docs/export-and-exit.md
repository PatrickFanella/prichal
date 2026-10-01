# Exporting data and preparing an exit

The local owner workspace exports a signed repository, its referenced media and bounded private Bluesky preferences. Version 2 archives add `preferences.json` to the version 1 portable format. This file contains the `app.bsky` namespace and exact stored preference values. It can include personal settings, so protect the whole downloaded archive even when the repository itself is public. Preference values are private application data and are not covered by the repository signature; the archive manifest and authenticated storage protect their integrity.

Preference capture reads only `name` and `valueJson` from the expected actor's `account_pref` table, through a read-only SQLite transaction. The actor path is derived from the verified DID and its SHA-256 prefix using the pinned PDS layout. No arbitrary path/table/query is accepted from management requests. Counts and UTF-8 bytes are checked before reading values. At most 1,000 preferences and 1 MiB of encoded preference content are supported. Names and stored `$type` must agree within `app.bsky`. Unsupported or oversized content fails capture without changing the source.

Existing authenticated version 1 artifacts remain unchanged on retry. Newly captured version 2 archives include the preference file even when its list is empty. Preferences and repository data use separate native reads, so they do not claim one shared atomic snapshot. Each CAR still represents one signed repository version; missing or inconsistent referenced media prevents publication.

## What the archive covers

- `repo.car` contains the account's signed public repository.
- `blobs/{CID}` contains media referenced by those signed records, with verified CIDs and hashes.
- `preferences.json` contains private Bluesky preferences stored by this PDS.
- `manifest.json` contains the account's public signing identity, capture time, archive format and member hashes.

Provider credentials, native account authentication settings, SQLite files and recovery keys remain outside the portable archive. Other app namespaces are not exported by this version. State in external services, such as chat messages and app-specific settings held elsewhere, needs that service's own export process. The [AT Protocol migration guide](https://atproto.com/guides/account-migration) distinguishes repository, blob and private-preference transfer and notes that external state may require separate handling.

## Preparing a migration

Download the owner-authorized archive before its 24-hour window closes. Keep its displayed SHA-256 and confirm that your copy matches. Protect it as private account data. On a designated destination, import the repository first and verify its record inventory, then upload missing referenced blobs and import supported preferences through the destination's authenticated APIs. The preference wrapper uses a `preferences` array suitable for the Bluesky preference API; its `namespace` is archive metadata. Other apps may define different APIs.

Data transfer does not change the active PDS in the DID document. Identity changes and destination activation require explicit account authority and additional verification. Retain the original hosting and recovery custody until the destination is verified. Public content already replicated elsewhere and external chat state are not erased by stopping a PDS.

Commercial cancellation, service-end dates, retention, reversal and confirmed deletion are not implemented by this export workflow. There is no scheduled source deletion. Live migration and production retention decisions remain separate gates for issues #21 and #29.

## Local evidence

The full local gate covers preference shape/namespace/size rejection, private encryption, version 2 reads and immutable version 1 replay. Actual drill `d4341ccf-c73b-4ffa-9cc3-581b6688218f` completed at 2026-10-01T13:29:46Z. Alice's normal export contained four signed records, one blob and one private preference. Bob's held export contained six signed records, one blob and one private preference. Independent verification matched each exact preference hash and signed/media inventory, and foreign owners were denied.

The disposable PDS fixtures have no configured Bluesky AppView preference API. The drill therefore seeded one marked preference directly in each otherwise empty local actor table, then removed only that exact marked value. This qualifies capture and download of the stored data; it does not qualify the public preference API. Native records/media, prior preference values and protected custody were retained. Bob's generation-25 hold was separately reversed at generation 26. Retained test archives include the marked synthetic setting.

See [local customer exports](local-customer-exports.md) for setup, custody and download bounds, and [local progress](progress/local-m1-alpha.md) for fresh-copy recovery evidence and remaining gates.
