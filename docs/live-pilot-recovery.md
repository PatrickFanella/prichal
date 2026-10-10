# Fresh pilot independent backup and populated restore

The disposable Dozor PDS passed a stopped native snapshot, independent encrypted backup on Kvant and populated offline restore on October 9, 2026 (October 10 UTC). This follows [public federation qualification](live-pilot-federation.md). PR #137 is merged at `7f1b60c34a759ae1de267de01e199a92e09f01d5`; the live node agent remains pinned to binary source `1d64932e4c9496b55834ccc62e06cf1268117e27`. Management production integration remains closed, and the existing NUC PDS is unchanged.

## Snapshot and encrypted custody

The host operator checked the exact pilot UUID/container, pinned PDS image, owned native data/blob volumes and mounted paths before pausing anything. It stopped only the pilot agent, edge and native PDS, captured the two native volumes plus private policy, PKI, agent state, account recovery credentials and unit configuration, then restarted those same resources in a `finally` path. The measured stop/snapshot/restart window was 2.93 seconds. The native container ID and source volumes were preserved.

The stopped snapshot is 481,280 bytes, SHA-256 `7e7b2045e92347651de688d1b2e3bd8c6a1aad914298280fcd0580947685e237`. It traveled over SSH to Kvant. The existing operator recovery implementation sealed it with AES-256-GCM, a fresh nonce and authenticated metadata identifying only the explicit `pds-pilot` scope and UUID `e8e82a65-7aa8-4e37-9306-0de701bf1b19`.

Kvant retains the mode-0600 ciphertext at `~/.local/state/prichal-pilot-recovery/20261010/native-snapshot.pdsbk` (481,436 bytes), SHA-256 `288b23d62beea3f9f439b35198f3aa29256ec08e6707606c546222ea6eb8d077`. Its separate mode-0600, 32-byte key is under `~/.local/state/prichal-pilot-recovery/keys/`. Both parent directories are mode 0700 and outside the development worktree. The key was never installed on Dozor or stored inside the backup. Recovery requires retaining this key as well as the ciphertext.

Authenticated decryption on Kvant recovered the exact snapshot hash. That recovered copy, rather than the original Dozor staging payload, was transferred back over SSH for the restore test. Temporary plaintext archives and staging payloads were removed after verification; ciphertext and receipts remain. No secret values or raw native environment are committed.

## Restore boundary and native results

The restore extracted authenticated archives using Python's `data` tar filter into new private directories under `/srv/apps/prichal/pilot/restore-20261010/`. Its pinned PDS container `prichal-pilot-restore-e8e82a65-7aa8-4e37-9306-0de701bf1b19` uses only those independent bind directories, `--network none`, zero published ports, restart policy `no`, fixed resource limits and no added capabilities. Its container ID is `ed2ffe5e0707c96d10955ee51889c87d4c42cd5ea7b3457f7fc4abba075a2b8a`. It has no public edge, relay or mail network access. The live pilot remains the sole public writer.

The [restore verifier](../deploy/pilot/check-restored-pds.cjs) receives only the operator's expected account/content information on private stdin. It refuses non-pilot configuration and any network interface beyond loopback before native calls. Its positive offline run passed; a dummy-input run in the networked live container was refused before any native call or credential input.

Inside the disconnected restored process, the verifier confirmed:

- Native account password login with the original DID, `prichal-test.subcult.tv` and confirmed email.
- Exactly one native repository.
- The exact test record value and CID, plus original blob bytes/hash.
- The same exported CAR hash and a valid repository signature against the real PLC signing key supplied as public verification data.

A separate custody comparison confirmed matching recovered agent credentials, TLS identity and immutable policy receipt.

The restore container is now stopped and retains network mode `none` and restart policy `no`. Its private restored data, host custody and reports remain for review. It is a recovery qualification fixture and must never be connected to the public edge or started as a competing PDS. It does not prove a full host-loss cutover, CA renewal, route restoration or management recovery.

## Operator crypto and replay

[The pilot crypto wrapper](../scripts/pilot-recovery-crypto.mjs) accepts only `seal`/`open` and three absolute private file paths. It fixes the pilot UUID and `pds-pilot` metadata. The shared decryptor checks the expected scope/instance before publishing plaintext, authenticates the complete envelope, and refuses to overwrite an existing destination. Local recovery scopes retain their existing behavior. The archive/envelope limit remains 512 MiB; this slice qualifies the small disposable snapshot and is not a large production backup service.

From a checkout containing the wrapper, open the retained backup into a new private mode-0700 staging directory:

```bash
rtk proxy node scripts/pilot-recovery-crypto.mjs open   /home/onnwee/.local/state/prichal-pilot-recovery/20261010/native-snapshot.pdsbk   ABSOLUTE_NEW_PRIVATE_STAGING_PATH/snapshot.tar   /home/onnwee/.local/state/prichal-pilot-recovery/keys/e8e82a65-7aa8-4e37-9306-0de701bf1b19.key
```

The staging path is a placeholder and must be replaced. Verify the plaintext SHA-256 against the receipt before extracting. Restore native databases/blobs and host custody into fresh isolated directories; confirm ownership, image and network constraints before starting any process. Use the retained private operator runners on Dozor only after inspecting subsequent changes. A rerun must select new staging paths and container names instead of replacing retained evidence. A public cutover requires a separate guarded procedure that stops the old writer and verifies key/identity, route, account/content and relay behavior.

## Verification and remaining work

`./scripts/dev.sh smoke` exited 0 with the pinned toolchains and separate `prichal-linux-qualification` PostgreSQL volume: 44 Node checks, web build, Go vet and all race-enabled packages passed. Crypto tests cover wrong keys, modified data/metadata, truncated envelopes, unsafe keys, no overwrite, nonce freshness and pilot scope/identity refusal before plaintext publication. The actual native restore and live-network refusal passed. Syntax and diff checks passed; canonical operations audit returned no integrity errors.

Post-maintenance authenticated agent status is healthy. The existing NUC PDS remains healthy at `0.4.5027`. Fresh public checks returned pilot health 200 at `0.4.5036` and both native XRPC and OAuth signup 403. Authenticated enrollment remained `verified` with confirmed email. Automated backup scheduling, independent recovery-key redundancy, full host cutover, private certificate renewal, reboot reconciliation, authenticated browser login and production management integration remain open. The private ingress certificate still expires October 17 at 00:56:38 UTC.
