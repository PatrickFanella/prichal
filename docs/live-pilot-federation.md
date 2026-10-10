# Fresh pilot handle and federation qualification

On October 9, 2026 (October 10 UTC), the disposable Dozor pilot completed public handle resolution, signed record/blob storage, public commit-stream observation and downstream Bluesky visibility. The PDS remains `https://prichal-pds.subcult.tv`; its agent still runs candidate `1d64932e4c9496b55834ccc62e06cf1268117e27`. PR #137 remains separate source review. The management preview and existing NUC PDS are unchanged.

## Handle ownership

The initial `pilot.prichal-pds.subcult.tv` handle did not resolve through Bluesky's public resolver. Almaz's existing DNS token returned Cloudflare error 9109, `Invalid access token`; no DNS record or credential was changed. The disposable account instead uses `prichal-test.subcult.tv`, a previously unused first-level hostname within the existing wildcard DNS/public TLS route.

Almaz's imported `prichal.Caddyfile` now serves the exact account DID from `https://prichal-test.subcult.tv/.well-known/atproto-did`. This follows the [AT Protocol HTTPS handle method](https://atproto.com/specs/handle). The route returns plain text and uses the existing public edge; it is not an additional PDS endpoint. See [the route artifact](../deploy/pilot/handle.Caddyfile). Full Caddy validation and graceful reload passed. The preceding file, candidate, validation/reload logs and hash receipt are retained privately at `~/.local/state/prichal-pilot-edge/handle-20261010/` on Almaz.

The native updateHandle operation changed only this disposable account's handle. Native login using the new handle retained DID `did:plc:gh4owm5uqgahod5wzoxqebhk` and `emailConfirmed=true`. The HTTPS ownership response and Bluesky's public resolveHandle endpoint returned that DID. The real PLC document's `alsoKnownAs` now includes `at://prichal-test.subcult.tv`, with the unchanged pilot PDS endpoint and signing key. Bluesky's AppView profile returned the correct DID and new handle.

## Record, blob and signature

The public native API uploaded one 68-byte PNG and created one labeled test post. The initial readable record key was refused by the pinned PDS's TID constraint. The retained test used a valid TID; the failed request did not create a second post. The operator's private runner stores the blob, record and key before issuance and reuses that state on replay.

- Record: `at://did:plc:gh4owm5uqgahod5wzoxqebhk/app.bsky.feed.post/3mxidphmumf22`.
- Record CID: `bafyreie4qhcgifgtoll4ar6txq47mbpsjns3adyev3nftikqparjssadvq`.
- Blob CID: `bafkreib27vtwe7e22fe2qsfo2iup4kee2stjnv2plpvspupe2v7pc5t7ve`.
- Native PNG SHA-256: `3afd67627c9ad149a848aed228fe2884d4a696d74f5beb27d1e4d57ef1767fa9`.
- Exported CAR SHA-256: `fce7533a7b17b4d683dc8bf9259233bc83b0fdd4f03f9b17b7ae0125405096dc`.

Native getRecord returned the exact written value/CID. Native getBlob returned the original bytes and hash. The public subscribeRepos WebSocket emitted the account's post commit at sequence 6. The [public-data verifier](../deploy/pilot/verify-repo.cjs), running inside the pinned PDS image, checked every exported block CID and verified the repository signature against the real PLC signing key. It found the test record, rejected a different signing key, and rejected an altered CAR block. No administrator key, password or bearer token is supplied to that verifier.

## Downstream and persistence

Bluesky's public AppView returned the exact post URI, record CID, author DID/new handle and blob CID. Its indexed timestamp is `2026-10-10T01:31:04.118Z`. The CDN served the image with HTTP 200 as a 256-byte WebP rendition; this transformed rendition is distinct from the original 68-byte PNG. No manual relay crawl request was needed for this observation.

The [public post](https://bsky.app/profile/prichal-test.subcult.tv/post/3mxidphmumf22) loaded in the T3 collaborative browser with the correct handle, test text and image alt text. The browser's AppView and CDN requests returned 200. This verifies anonymous public viewing; interactive authenticated client login remains a separate test.

Restarting only the pilot's native PDS container preserved native handle login, confirmed email, record CID/value, blob bytes/hash and exported CAR hash. Enrollment replay remained `verified`. Both public signup paths returned 403 afterward. The NUC health endpoint still returned version `0.4.5027`.

## Custody and remaining gates

Dozor retains private `content-state.json`, account state, CAR export, stream/signature/content/federation reports and the operator runners under `/srv/apps/prichal/pilot/`. Selected public-data reports are copied to ignored `.local/pilot-artifacts/` in the development checkout. Credentials and verification codes are absent from tracked artifacts.

The pilot now has real account/email qualification and one end-to-end federated record/image. Independent native backup and populated restore, automated private certificate renewal, reboot reconciliation and management production integration remain open. The ingress certificate expires October 17 at 00:56:38 UTC. The unrelated invalid edge DNS credential remains unrepaired.

For handle-route rollback, inspect changes after the retained Almaz hash receipt before restoring the preceding imported file and gracefully reloading validated Caddy. Removing ownership verification alone invalidates the new handle; coordinate a deliberate native handle recovery first. Do not change the NUC route, delete the pilot's keys or activate a second writer. Keep the labeled public record as recovery-test data until the populated restore is qualified.
