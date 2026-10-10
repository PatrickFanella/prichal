# Fresh Dozor pilot evidence

The separate PDS at `https://prichal-pds.subcult.tv` was provisioned by the restricted Prichal node agent on October 9, 2026. The existing NUC service at `pds.subcult.tv` remains unchanged. The management preview still runs `decd4442c8dba11b101c7165213fcfaec67b06ed` and cannot deploy or enroll this live instance.

## Installed bootstrap

- Agent source: merged PR #136, `b493acad6d69f9bbe7528d87d6988c0afd426b88`.
- Static binary SHA-256: `b3aa57061702842f7450278c26d3076563ea12a399d5c299c1222eea7ce5e45a`.
- Instance: `e8e82a65-7aa8-4e37-9306-0de701bf1b19`, native PDS `0.4.5036`.
- State: `/srv/apps/prichal/pilot/state`; immutable private policy and agent environment: `/etc/prichal/pilot/`; dedicated CA and mutual TLS certificates: `/etc/prichal/pilot/pki/`.
- Dozor user unit: `prichal-pilot-agent.service`, loopback `127.0.0.1:9091`; enabled with user lingering. See [installed unit](../deploy/pilot/prichal-pilot-agent.service).
- Native PDS has separate labeled data/blob volumes and no published port. Its pinned Caddy edge publishes only `127.0.0.1:45624`.
- Almaz user unit: `prichal-pilot-edge-tunnel.service`, management bridge `172.20.0.1:34943` to Dozor's private edge. SSH uses a dedicated foreground connection and strict retained host authentication. See [installed tunnel unit](../deploy/pilot/prichal-pilot-edge-tunnel.service).
- Almaz's existing imported `prichal.Caddyfile` contains the dedicated HTTP route behind Cloudflare. Its HTTPS upstream pins the exported private certificate, SNI `e8e82a65-7aa8-4e37-9306-0de701bf1b19.localhost` and public Host header. The main Caddyfile remained byte-identical; complete validation and graceful reload passed.
- Ingress certificate fingerprint: `85:F4:0F:35:A2:F5:05:78:C5:36:4D:BE:A9:47:09:6F:DE:BF:60:40:B8:70:6E:D7:55:6C:9E:C3:64:FB:AF:C6`; expires October 17 at 00:56:38 UTC. Private certificate renewal is required before expiry and is not automated.

## Runtime checks

Public requests with normal certificate verification returned health 200, version `0.4.5036`, and an empty native repository inventory. Native account creation and OAuth signup returned 403. Unknown private Host returned 421. `describeServer` returned the selected service DID, required invitations and a 5 MiB blob limit. The separate PDS's `/.well-known/did.json` probe returned 404; discovery and downstream federation remain unqualified.

Authenticated ensure/status succeeded for the approved UUID. A wrong UUID and enrollment mutation returned 404; missing client certificate was rejected during TLS authentication. Restarting the agent and repeating ensure retained the exact PDS/edge container IDs and credential, policy and TLS hashes. SMTP TLS and authenticated login succeeded without sending mail. Neither this check nor sender configuration proves inbox delivery.

Private reports are retained under `/srv/apps/prichal/pilot/` on Dozor and ignored `.local/pilot-artifacts/` in the development checkout. Almaz retains its route backup, candidate and validation receipt under `~/.local/state/prichal-pilot-edge/`. These directories contain runtime or private state and must not be committed.

## First-account live qualification

PR #137 candidate source `1d64932e4c9496b55834ccc62e06cf1268117e27` was built with the pinned toolchain and installed for the disposable live test. The binary SHA-256 is `bee0eeb925a9d4326d87f342dc76795ea58eba84edc2098ff5a934c08315abee`. The [enrollment drop-in](../deploy/pilot/enrollment.conf) selects that pinned release and literal host opt-in; the base unit still retains the preceding merged bootstrap. This candidate deployment does not establish a merge to main or a management deployment.

The preparation helper successfully retained the closed edge under its `-before-pilot-enrollment` name and the preceding Caddyfile. Reconciliation created the first-account-only guard and replacement edge. The native PDS container ID and credential, policy and TLS hashes remained identical to the bootstrap custody receipt. Agent restart and authenticated ensure retained healthy status.

A private host controller opened a fifteen-minute invitation lease. The public native createAccount endpoint created `pilot.prichal-pds.subcult.tv` with DID `did:plc:gh4owm5uqgahod5wzoxqebhk`. The real PLC directory returned its document with service endpoint `https://prichal-pds.subcult.tv`, and the public repository inventory returned exactly this active account. XRPC and OAuth signup returned 403 afterward, and enrollment replay returned the existing account without an invitation. The initial default Python client received a non-JSON 403; the explicit qualification client succeeded. Interactive browser and other clients still need verification.

The owner selected a recipient privately in the development thread. The PDS accepted `requestEmailConfirmation` after account creation. The owner completed inbox confirmation, and a fresh authenticated enrollment read on October 9 returned native `emailConfirmed=true` and state `verified`. The recipient, generated password, session tokens and verification code are absent from tracked documentation. Private account state is mode 0600 under `/srv/apps/prichal/pilot/`, with an operator-only `confirm-email.py` that reads a hidden terminal input and submits the code directly to the public PDS. It checks native `emailConfirmed=true` after confirmation. No code is logged or sent through management.

The initial default-domain handle was later replaced with the publicly resolved `prichal-test.subcult.tv`. Signed record/blob, commit stream, Bluesky AppView/CDN and anonymous browser checks now pass for the test post. See [handle and federation evidence](live-pilot-federation.md).

## Recovery and remaining gates

To remove the public pilot, restore only its preceding imported route file and gracefully reload validated Caddy, then stop its dedicated Almaz tunnel. The existing NUC route does not change. Stop the Dozor pilot edge and agent as needed, retaining policy, keys, certificates and volumes together. Avoid a second native writer. The first-account upgrade retains the preceding closed edge and Caddyfile separately for rollback.

The PDS, edge and guard use restart policy `no`. User units restart the agent/tunnel; the agent does not automatically reconcile containers after a host reboot. Explicit authenticated ensure and health checks are required. Reboot recovery, automated certificate renewal, independent backup, populated restore, authenticated browser login and management production integration remain open. This is a fresh pilot deployment, not a completed SUBCULT identity migration or production management release.
