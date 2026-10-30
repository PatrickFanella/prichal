# Staging-02 endpoint withdrawal and native retirement

This unit continues the owner-authorized retirement of the empty staging-02 fixture after PR #217 merged the schema-42 management record as `5293a29131cf57426cf7337f45b9906cbe9b3599`. It adds the two host-side steps the management `release` phase requires receipts for: an exact public-route withdrawal on Almaz and a held native container removal on the guest. Both tools are fixed to deployment `2395105c-9958-47b5-ae8b-5b0424cbfc4f`, hostname `pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv` and forward 35000. Neither is an API or worker route.

## Edge withdrawal

`deploy/homelab/publish-managed-staging.py` gains `prepare-withdrawal` and `withdraw`. Both require root on Almaz and `--confirm-staging-02`. Preparation takes only the currently applied publication revision. That revision's retained inventory must contain the designated fixture as its sole endpoint, and the installed parent/route bytes must match its receipt.

The publisher derives the post-withdrawal parent from the initial publication's retained `parent-before` bytes. It walks the update chain back to that revision and requires that re-adding the owned import reproduces the installed parent exactly. The candidate main configuration is adapted and validated in a private staging directory. Preparation refuses if the candidate still routes the hostname or is identical to the active configuration. Nothing live changes during preparation.

`withdraw` rechecks unrelated sources, owned files, the preceding applied publication, active configuration and that no other withdrawal has started. It writes a `withdrawal-started.json` tombstone before the first live change. It then restores the parent, removes the managed route file, validates and adapts the live files against the prepared candidate, reloads with the root ETag and reads back an active configuration without the hostname. A failure after the first file change runs the hash-bound rollback without re-verifying the native upstream, because it restores the configuration that was active moments earlier. An interrupted process keeps its tombstone and resumes by replaying `withdraw`. Success writes `withdrawal-receipt.json`, which records the revision chain, owned file digests and active before/after digests, then the history marker. Its SHA256 is the `--withdrawal-sha256` input to management release. Exact replay returns the same digest without reloading.

A started withdrawal is a publication tombstone. Later preparation or publication refuses any inventory containing the withdrawn deployment ID or its forward port, even with a newer generation. Only a rollback of the withdrawal revision itself removes the tombstone. An operator-requested rollback re-verifies the preceding revision's pinned native upstream and refuses once the native containers are gone. The withdrawal receipt is not public absence proof. Operators still need an external request showing the unknown-host response.

## Native hold and removal

`deploy/homelab/retire-staging-02-managed.py` runs as root on guest `prichal-staging-02`. It requires `--confirm-empty-staging-02`, `--confirm-remove-native-containers`, a new request key, the edge withdrawal receipt digest, an inactive agent and the agent's exclusive state lock. It verifies the [preservation](managed-fixture-retirement.md) report and archive digest `1b6617c6068fc8dfaa45e0c8cda34f9a09293684e63c89e6c91c41301fe4a56f`. The labeled namespace must contain exactly the four preserved container IDs with disabled restart, generated identity, closed registration and the two named volumes. A running fixture must report zero accounts; a stopped one relies on the preservation report's zero-account verification.

The tool first installs an agent service lease with state `suspended`, phase `applied` and generation 2⁵³. That is the agent's maximum, so `Ensure`/`EnsureManaged` treat the instance as held, and any different later service request is refused as stale or conflicting before a runtime call. An identical replay of the hold passes that fence and then fails the read-only runtime inspection because the containers are gone. A Go regression covers this exact lease. The tool then stops the edge and writer and removes the four exact container IDs without `-v`. Data/blob volumes, networks, native credentials, TLS, managed identity, node key/certificate and preservation custody remain. The receipt records removed IDs, retained volume mountpoints, retained-file digests, the hold digest and the edge-withdrawal digest. Its SHA256 is the `--native-retirement-sha256` input to release.

The before-state (containers, volume mountpoints and retained-file digests) is written atomically, and a stale temporary file from an interrupted write is replaced. An interrupted run resumes under the same request key from that retained before-state. Exact replay verifies that the namespace is empty, volumes and retained files are unchanged and the hold is intact. A different request key refuses if the containers are already absent. Refusals before the hold leave containers untouched. This does not release capacity; management release separately measures the empty authenticated namespace.

## Live order

1. Verify node transport validity. The retained-key certificate expires October 30 at 22:18:46 UTC; renew it through the [retained-key procedure](staging-02-transport-renewal.md) before any authenticated step after that.
2. Take a fresh management snapshot, upgrade the dedicated registry to schema 42 with compatible binaries and run `managed-fixture-retire prepare` while the fixture is still healthy.
3. On Almaz, run `prepare-withdrawal` against the currently applied publication revision, then `withdraw`. Confirm from outside that the hostname now receives the unknown-host response.
4. Stop only the two dedicated managed forwarding units and preserve their files.
5. On the guest, stop the agent, run the retirement tool with the withdrawal digest and restart the agent.
6. Run `managed-fixture-retire release` with both receipt digests.

Before step 5, withdrawal rollback restores the route. After step 5, recovery means removing the hold deliberately and recreating containers from the retained volumes and credentials, or restoring the preserved archive. Neither path is qualified here. Do not delete the hold, volumes, keys or receipts to free capacity.

## Verification

Eight publisher withdrawal tests cover full withdrawal to the exact initial parent, tombstoned identity/forward, replay, operator rollback refusal without a current native upstream, injected reload failure with restoration (including with a degraded upstream), an interrupted process that stays tombstoned and resumes, refusal of a second prepared withdrawal, unrelated/owned drift refusal and foreign scope/changed install/tampered chain refusal. Four native tests cover hold-then-remove with retained volumes/files, replay, changed-digest/credential refusal, resumed partial removal, replaced partial custody files and seven pre-mutation refusals. The Go regression proves the agent cannot supersede the hold.

These are local checks with fakes for Caddy and Docker. No live registry, route, forward, guest container or capacity changed in this source unit.
