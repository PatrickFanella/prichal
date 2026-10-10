# Restricted pilot agent

The node agent has a separate `PDS_MODE=pilot` for one fresh operator-selected PDS. This is the host runtime slice of the Dozor pilot. Production deployment and enrollment in the management API and worker remain closed. The existing NUC PDS is not adopted or changed.

## Host policy

Place a mode-0600 JSON file outside a new, mode-0700 agent state directory. It contains exactly these fields:

```json
{
  "instanceId": "OPERATOR_SELECTED_UUID",
  "hostname": "prichal-pds.subcult.tv",
  "emailFrom": "OPERATOR_SELECTED_SENDER",
  "smtpUrl": "smtps://ENCODED_USER:ENCODED_PASSWORD@MAIL_HOST:465"
}
```

These are placeholders, not deployed credentials or a selected account. The hostname must be a plain public DNS name; `pds.subcult.tv`, IP addresses and local testing names are refused. SMTP requires authentication and implicit TLS on port 465. STARTTLS and URL options that could disable certificate verification are not supported by this first policy.

On first startup the agent persists a private SHA-256 receipt of the canonical policy before it serves requests. It refuses a nonempty state directory without that receipt. A restart with the same policy retains the receipt and identity keys; changing any policy field fails startup. SMTP credential rotation therefore requires an explicit later policy-update workflow. Do not delete the receipt to bypass that restriction.

Before generating the first identity keys, reconciliation also refuses preexisting volumes, a tenant network or ingress in the selected namespace. It can resume its own interrupted resource creation once those keys exist. Missing or unsafe credentials for retained resources fail reconciliation. This is not an existing-PDS adoption workflow.

## Startup and private control

Build `backend/cmd/node-agent` with the pinned Go toolchain. Provide these host-owned environment settings through private runtime configuration:

```text
PDS_MODE=pilot
PDS_AGENT_LISTEN=127.0.0.1:9091
PDS_AGENT_DOCKER_SOCKET=/var/run/docker.sock
PDS_AGENT_STATE_DIRECTORY=/srv/apps/prichal/pilot/state
PDS_AGENT_PILOT_POLICY_FILE=/etc/prichal/pilot/policy.json
PDS_AGENT_CA_FILE=/etc/prichal/pilot/ca.pem
PDS_AGENT_CERT_FILE=/etc/prichal/pilot/agent.pem
PDS_AGENT_KEY_FILE=/etc/prichal/pilot/agent-key.pem
```

The paths illustrate the intended separation. The installed Dozor pilot uses a private `pki/` directory as recorded in [live bootstrap evidence](live-dozor-pilot.md). The agent listener must be a loopback IP. Its dedicated CA must verify the client certificate with common name `pds-ass-worker`; TLS 1.3 and mutual certificate authentication remain required. Do not give the API the socket, host credentials, policy file or client private key.

The pilot handler accepts only `PUT /v1/instances/APPROVED_UUID` with `{}` and `GET /v1/instances/APPROVED_UUID`. Other UUIDs, runtime configuration, query parameters, member invitations, moderation, export and service mutations are unavailable. Enrollment is unavailable by default; the explicit host opt-in below enables only fixed first-account enrollment and withdrawal routes. A host-side controller can use the dedicated client certificate to exercise these two routes. Existing management clients still validate local endpoints and cannot drive this pilot.

## Runtime and ingress

The agent retains the pinned PDS and Caddy images, separate labeled data/blob volumes, private identity files and resource limits. It clamps capacity to one slot while retaining measured host limits and accounting for other managed namespaces. Reconciliation checks the tenant network's ownership, bridge driver and outbound connectivity setting. The bridge permits outbound traffic for real PLC, relay and mail services; it does not impose a destination allowlist.

The PDS uses the selected public hostname and service DID, `PDS_DEV_MODE=false`, required invitations, real PLC, Bluesky relay, AppView and report-service configuration. The PLC, relay, AppView and report-service values follow the [official PDS installer](https://github.com/bluesky-social/pds/blob/main/installer.sh). A configured service is not evidence that a request succeeded.

Caddy listens inside its container on 8443 and publishes only to `127.0.0.1` on the UUID-derived qualification port. Pilot validation refuses a changed host binding. It accepts the selected hostname and the private UUID localhost name, rejects unknown hosts with 421, and returns 403 for native account creation and OAuth signup unless a current first-account lease admits the request. The PDS itself has no published port.

The agent's health probe uses the retained private localhost TLS identity. The returned instance endpoint is the public hostname, but `healthy=true` proves only private ingress and native process health. It does not prove DNS, public TLS, mail or federation. The private certificate still has the existing seven-day qualification lifetime; renewal is not implemented here.

## Deployment boundary

The fresh Dozor pilot now has a dedicated user service, private SSH edge tunnel and public route. See [live bootstrap evidence](live-dozor-pilot.md). Production node enrollment and worker placement remain closed. Preserve the NUC route and service throughout.

With bounded first-account enrollment enabled, verify a native account, email confirmation, signed record/media and downstream visibility. Prove identity custody and a native restore before important accounts or invitations. Keep the pilot's policy, credentials, TLS identity and volumes together for recovery; rolling back code does not justify regenerating its keys or starting a second writer.

## First-account host opt-in

`PDS_AGENT_PILOT_ENROLLMENT=true` enables only `POST /v1/instances/APPROVED_UUID/enrollment` and `/enrollment/withdraw`, with a body containing one UUID `ticket`. The setting is refused outside pilot mode and accepts only the literal `true`. The existing mutual TLS and worker certificate checks still apply. It does not enable the management worker, general invitations, account administration or runtime configuration.

Enrollment uses the existing fifteen-minute lease, single-use native invitation, retained ticket replay and drained replacement/withdrawal flow. The production helper checks the exact pilot hostname, service DID, real PLC, disabled development mode and TLS SMTP before touching invitations. Native passwords and verification tokens go directly to the PDS. The guard ignores all member admission policies in pilot mode and refuses signup once native repository inventory is populated. A configured lease is not proof of a completed account or delivered mail.

An existing closed pilot ingress fails reconciliation against the new guard policy. Stop the pilot agent, then run `node scripts/prepare-pilot-enrollment.mjs ABSOLUTE_STATE_PATH ABSOLUTE_POLICY_PATH` as the agent's host user. The helper verifies the policy receipt, pinned tenant/edge images, production configuration, private mounts and loopback port. It stops and retains only the pilot edge container and its Caddyfile. It leaves the native PDS, data/blob volumes, credentials and TLS identity in place. Enable the host setting, start the chosen agent revision and reconcile the same UUID. Without a lease the new guard must return 403 on both signup paths.

For rollback, stop the agent and new pilot edge/guard. Retain those containers under distinct names, retain the new Caddyfile, restore `Caddyfile.before-pilot-enrollment` and the edge container named `pds-ass-edge-UUID-before-pilot-enrollment`, remove the host opt-in, and start the preceding pinned agent revision. Restore the original edge name and start it. Verify both signup paths remain closed and health recovers. Never restore a second native PDS writer or regenerate keys. The inspection receipt is private host state; it must not be committed.
