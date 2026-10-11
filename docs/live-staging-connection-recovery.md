# Connected staging checkpoint recovery — October 11, 2026

**PASS: isolated recovery of the connected Kvant staging node and its management registry.** The independent Dozor checkpoint and independent recovery-key copy recovered the managed PDS runtime, registered identity, retained placement, operator/owner access and applied service generation. The PDS has zero accounts; this is not populated account recovery or public cutover.

PR #144 is merged at `400396df28cd465bfea02e41c454a7fa4bfc4f3d`, with a successful main smoke check. The live management runtime continues using source commit `12bda879` and its pinned image. No production management release changed during this qualification.

## Independent source and isolation

The checkpoint is Dozor's `/srv/recovery/kvant/prichal-staging/staging-connection-20261011.pvm`, ciphertext SHA-256 `7c6195a3f3f0f38b0716584537b64b2ab8a12882569718f65e4d0883fdfaab78`. Its separate private key was retrieved from Dozor's existing recovery-key custody. AES-256-GCM authentication and the trusted plaintext digest `4637cecc01948adc9c633a91c0b97493f3eb9753dc34525d5a30cf576286f7e2` passed before extraction. The archive contains 96 members. Only the fixed VM/control/management/recovery paths are allowed; absolute paths, traversal, links and special files are refused.

The fresh fixture root is `/mnt/fast1/apps/prichal-connection-restore-20261011`, mode 0700. Its independent 40 GiB disk passed `qemu-img check`. A transient, non-enabled user service booted the restored three-CPU/4 GiB guest with QEMU `restrict=yes` and only loopback SSH port 22253. No management agent tunnel was installed. The original SSH host key was pinned at the fixture port. A guest connection attempt to external TCP 443 was blocked.

Management restoration used project `prichal-connection-restore`, an independent PostgreSQL volume/database and Docker network marked `internal=true`. None of its containers published ports or used host networking. API, disposable issuer and restored frontend share a bounded sandbox network namespace. No worker or observer runs in this fixture; registry endpoints pointing to the live node cannot cause reconciliation. An external TCP connection probe from that namespace was blocked. API mounts use copied management keys and copied artifacts only, without node, signer, SSH or Docker credentials.

The saved management image was loaded from the recovered archive and returned the original immutable image ID `sha256:15685feeb202b34af17583c3bbb0d7e3142093be422462f0fc31d1d6ebdbd21f`. Node is pinned to the locally cached image ID for the fixture. No new image pull was required.

## Observed recovery

Authenticated reconciliation was sent only inside the restored guest, using the restored authorized worker client files. It started retained native container `ef37ab59e4cc` and edge `51c5e82a2ad0`. Subsequent status reported running/healthy, applied service generation 2 and retained resume ticket `1db17edc-cbb0-46b4-803c-1b1415259e61`. Native TLS health returned version `0.4.5036`; native usage reported zero accounts.

The restored guest certificate exactly matches the restored registry certificate. The SHA-256 of its public key matches the registered node pin for `1ef77d45-c6d5-4bdb-b77b-ba1103b45a66`. PostgreSQL retains one node, one placement, one consumed enrollment credential and two applied service operations. Placement still connects deployment `53a1f2d3-181b-4cf2-b242-2d35ca55c784` to that node.

Through the restored frontend/API and disposable issuer, Alice sign-in succeeded and returned the retained ready deployment. Operator inventory showed one reserved slot and refused to treat the old capacity observation as fresh. Bob could read neither operator inventory nor Alice's deployment. CLI bearer inventory access returned 403. This verifies restored local authentication/authorization state, not production OIDC.

The first fixture API start preceded its issuer and failed OIDC discovery. The final manifest uses a separate network-namespace anchor; start the issuer and verify discovery before starting API. No live service was restarted to resolve this fixture ordering.

## Procedure and retained state

The fixed dated operators deliberately refuse an existing fixture directory. Preserve the completed fixture; do not overwrite it to repeat qualification. Future checkpoints need their own reviewed identity, hashes and paths.

1. Use `deploy/homelab/prepare-staging-restore.py` to retrieve/authenticate the independent archive and key, validate extraction scope, copy reviewed VM operators and write private fixture configuration. It verifies the required SSD mount and refuses existing state. The live acquisition/preparation sequence passed; the subsequently packaged operator also passed syntax, archive-scope tests and existing-directory refusal.
2. Check the extracted disk with `qemu-img check`. Launch the copied `restore-staging-vm.py launch` under transient `prichal-connection-restore-qualification.service` with mode 0077, 5 GiB memory limit and 300% CPU ceiling. The helper accepts only the fixed independent disk and outbound-restricted network.
3. Start only fixture PostgreSQL with `restore-management.compose.yaml`. After `pg_isready`, pipe the recovered logical dump to `psql -v ON_ERROR_STOP=1` in `prichal-connection-restore-postgres-1`; keep SQL output in a private log. Never restore over the live registry.
4. Start sandbox/issuer/web. Verify issuer discovery from inside the namespace, then start API. Run `node /checks/qualify-restored-staging-management.mjs` inside fixture web; it performs no provisioning or entitlement writes.
5. Through pinned fixture SSH, create a private `/tmp/prichal-restore-client` directory and copy only restored worker client key/leaf/public CA. Send authenticated `PUT /v1/instances/53a1f2d3-181b-4cf2-b242-2d35ca55c784` to guest loopback 9091. Export only the public native certificate to that scratch directory and verify native TLS health. Run `qualify-restored-staging-node.py` after readiness; it checks the running guest's isolation, registry pin/certificate, retained runtime IDs, placement, generation and usage.
6. Remove only the fixture scratch client files. Cleanly shut down the restored guest with `restore-staging-vm.py powerdown --pid <verified fixture PID>`; it refuses a foreign process. Stop this Compose project without removing its volume. Remove generated plaintext archive, SQL dump and image export after verification. Retain private stopped fixture disk/state and receipts.

The restored guest is now cleanly shut down and all five management fixture containers are stopped. Its database volume is retained. Generated plaintext archive/dump/image exports were removed. Private reports remain at the fixture root. The encrypted source checkpoint and separate recovery-key copies remain intact.

Fifteen homelab operator tests, Python syntax, Node syntax and Compose configuration checks passed. Application code was unchanged, so the full application smoke gate was not repeated locally; merged PR #144/main had a successful hosted smoke check. The new infrastructure PR has its own hosted gate.

## Live continuity and remaining gates

Fresh NUC, Dozor pilot and private staging native health returned `0.4.5027`, `0.4.5036` and `0.4.5036`. Kuznya and live staging VM PIDs remained 930757 and 1994020. The live management observer, certificate timer and placement remain independent of the stopped restore fixture.

Public staging routing, production sign-in/management integration, populated account recovery, scheduled checkpoint acquisition, naturally due certificate renewal/alerts, unattended native reconciliation after guest boot and physical host-loss cutover remain unqualified. This test recovers an isolated copy; it does not authorize replacing a live registry, enrolled identity or route with checkpoint state.
