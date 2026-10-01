# Local customer export contract

The local implementation exports a portable signed repository CAR and its referenced media. This is distinct from the protected operator recovery archive, which contains native SQLite, host credentials and shared fixture state. Customers receive only their own native account's portable data, public signing identity and a manifest.

Management authorization selects a ready owned deployment and its verified native DID. A durable, idempotent export request and River job commit together. The worker alone calls the registered restricted agent with a fixed instance/DID contract. The pinned native helper requires the sole retained actor to match that DID, verifies CAR signatures and each referenced blob, and excludes private runtime/account settings. Account takedown retains operator export eligibility; service suspension leaves the export pending or failed without deleting data.

Capture and lifecycle execution serialize on the deployment row and restricted agent. Native user writes may continue; the CAR describes one signed repository version, and every referenced blob must match before publication. Missing or changed content fails capture instead of publishing a partial bundle.

The local artifact store uses a separate private AES-256 key, authenticated encryption bound to export/deployment/DID identifiers, private files and atomic publication. The worker reuses a valid existing artifact after a lost management checkpoint. The API has access only to management state and the encrypted export store, never Docker or host credentials. Owner-authorized downloads decrypt and verify completely before responding with a fixed-name tar archive.

Local limits bound CAR/media and total archive size, retained requests per deployment and download lifetime. Expiry closes downloads; it does not delete source or retained artifact data. Limits and local qualification do not establish a commercial cancellation or retention policy. Cancellation, independent artifact storage/key custody, streaming larger datasets, live download qualification and actual account migration remain separate gates.
