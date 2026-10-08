# Fresh live Prichal PDS pilot

Decision recorded October 8: the owner accepts either NUC or Dozor and prefers a fresh test PDS if that makes testing easier. The first live test will use a separate PDS on Dozor, alongside Prichal's management preview. It requires no import of the existing SUBCULT account data. This changes the test target, not the public-release gate.

## Existing service

Read-only checks on October 8 found `subcult-pds` healthy on NUC, serving official PDS version `0.4.5027`. Its Compose owner is `/srv/repos/subcult/subcult-pds`; its data bind mount is `/srv/data/subcult-pds`, and its LAN listener is `10.0.0.56:3043`. The public endpoint is `https://pds.subcult.tv`. SUBCULT's PLC document still identifies `at://subcult.tv` and that endpoint.

The native actor inventory has nine entries, including SUBCULT, Patrick, CLPR, another personal account and test accounts. Treating the SUBCULT data as disposable does not require deleting this whole service. Keep it running while the separate pilot is qualified. Any later retirement should name the accounts and data being retired. No production account, container, route, key or volume was changed during this assessment.

## First live test

Use one operator-controlled test account and one fixed hosting slot. A first-level hostname such as `prichal-pds.subcult.tv` avoids introducing a second-level wildcard requirement; confirm the actual edge route and certificate before selecting it. Do not replace the current `pds.subcult.tv` route. Use a separate controlled test handle and real PLC/email services. Store runtime credentials on the host and preserve the separation between the management API, worker and restricted node agent.

Required deliveries, in order:

1. Fix the native Linux private-file readers and verify first-account/member journeys locally (#125).
2. Add an explicit production node policy and private enrollment/bootstrap for Dozor (#3, #6). Bind it to the designated hostname, storage namespace, image, capacity and host identity. The current agent and worker require local mode; removing their guards alone is not a production implementation.
3. Add the selected public endpoint and fixed ingress route, verify DNS/TLS and reject unknown hosts (#7, #126). The current runtime derives localhost endpoints and local TLS identities.
4. Enable a bounded operator enrollment path with real `plc.directory` and the configured mail sender (#8). Management retains invitation state; native passwords and email tokens continue to go directly to the PDS.
5. Verify native login, email receipt/confirmation, a signed record and media, then downstream visibility in Bluesky (#9). Record the actual operation and account identifiers and all failed attempts. A public container health check does not complete this gate.
6. Prove identity-key custody and a complete native restore before using an important account or inviting other people (#11, #12). Harden the management preview's backups, admission and monitoring alongside this work (#127).

The deployed management preview disables production deployment/enrollment; the worker and agent require local mode, and their runtime policy has no existing-PDS adoption path. A separately started official PDS would test public hosting, but it would not prove that Prichal provisioned and manages it. Preserve that distinction in the next delivery.

## Later use of the SUBCULT identity

After the pilot passes, the existing identity can move through the official account migration process, or the whole existing PDS can be brought under an explicitly supported adoption policy. Keeping the same endpoint for a whole-service relocation avoids an endpoint change, provided all databases, blobs and identity keys are retained consistently and only one copy writes. Moving an account to a different endpoint requires identity verification, repository/media transfer, an explicitly checked PLC update and activation/deactivation steps. See the [official account migration guide](https://atproto.com/guides/account-migration).

Prichal currently offers read-only local migration inventory, not a qualified account import/cutover workflow. Fresh testing avoids making that unfinished workflow a dependency of the first live PDS.
