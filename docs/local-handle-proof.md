# Customer handle instructions and proof

An owner with a verified first account enters a domain hostname. Management records this intent and shows the exact `_atproto.HANDLE` TXT name and `did=ACCOUNT_DID` value. No registrar transfer or PDS replacement is required.

After publishing the record, the owner changes their handle through an authenticated native account client. Management never accepts a native password or access token, signs a handle change, or uses the hosting administrator as the account owner. Checking instructions cannot change the native account.

Verification uses the protocol's preferred TXT method. It requires a unique account DID claim, the matching `at://HANDLE` reverse alias, and one PDS service entry at the deployment's retained endpoint. Missing DNS stays pending; ambiguous or invalid claims conflict. Directory failures remain unavailable. TXT records for the same DID can repeat, but differing DIDs are rejected. The directory URL is fixed by the operator, responses are bounded and redirects are refused. Customer hostnames do not cause HTTP requests.

Rechecks are limited to one per deployment every 30 seconds, including across domain changes. A revision fence discards results if a newer domain request arrives during resolution. Removing DNS and checking again reports loss of proof. To change or undo a handle, authenticate to the native account, change its handle and update the management instructions. Endpoint and storage lifecycle remain independent.

Local mode permits `.test`; live handle registration must reject it and other reserved namespaces. Production routes remain closed pending qualification. The first implementation supports TXT instructions and checking. HTTPS well-known resolution and an authenticated browser shortcut for native changes are later work.

```sh
node scripts/qualify-handle-instructions.mjs DEPLOYMENT_UUID
```

This local command checks exact instructions, owner scope, repeated intent, rejected credential fields, bounded rechecking, pending DNS and unchanged deployment state. It does not attach a controlled domain. Disposable PostgreSQL and resolver tests cover successful bidirectional proof, DNS delay and removal, conflicting and malformed DIDs, identity outages, endpoint conflicts and an in-flight domain change. A controlled-domain native attachment and removal remain required for issue #10.

Protocol reference: [AT Protocol handle specification](https://atproto.com/specs/handle).
