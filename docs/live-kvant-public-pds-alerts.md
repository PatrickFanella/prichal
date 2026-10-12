# Public Kvant PDS certificate monitoring — October 11–12, 2026

The public pilot's expiry and renewal receipts now reach independent Dozor Prometheus through a restricted SSH publisher. Ten samples cover four role expiries, inspection/result status, latest attempt/success, source observation and Dozor receipt time. Dozor can detect missed publications while Kvant is unavailable. The earlier Dozor pilot's telemetry and all existing alert mutes remain separate.

## Publisher and receiver

Kvant's `prichal-public-pds-metrics.timer` runs twenty seconds after activation and every five calendar minutes. Its private source, dedicated SSH identity, pinned existing Dozor host trust and bounded state are under `~/.local/state/prichal-kvant-public-pds-metrics/`. It reads only the fixed `last-certificates.json` receipt. It neither reads signer/worker/guest private keys nor requests Docker/native execution. Failed or corrupt receipts preserve last known expiry/success while publishing failure; a successful publication never advances the renewal success timestamp by itself.

The new Dozor authorization is `restrict,command="/usr/bin/python3 /usr/local/libexec/prichal-kvant-public-pds-metrics.py"` for this dedicated key. All preceding authorizations were preserved. The root-owned receiver runs as `onnwee` and can write only its fixed metrics file; it has no path/command arguments, rejects a supplied SSH original command, and accepts a bounded exact schema for the fixed pilot ID and four numeric roles. It rejects stale/future observations, nonfinite values, unknown fields and symlink targets. The live publisher worked and an `id` request through that key was refused.

The atomic metrics file is `/var/lib/prometheus/node-exporter/prichal-kvant-public-pds/certificates.prom`, owner `onnwee`, mode 0644. A root-owned symlink in the existing textfile collector directory exposes it. Existing Node Exporter was preserved; the private key stays on Kvant. Publisher state is mode 0600 and retains the actual last successful renewal inspection timestamp.

## Rules and runtime

`public-pds-certificate-alerts.yml` installs four rules through the existing explicit Prometheus rule-file list:

- Certificate expiry within two days, sustained fifteen minutes, warning.
- Failed inspection for five minutes, critical.
- Missing telemetry or a publication older than fifteen minutes, sustained ten minutes, warning.
- Failed renewal result or a success heartbeat older than two hours, sustained ten minutes, warning.

Rules identify `project=prichal`, `host=kvant`; expiry retains the role label. Zero/unknown expiry is covered by inspection failure rather than an invented expiration time. These labels are outside the existing retirement mutes. Warning uses the existing `ntfy-no-resolved` receiver; critical uses existing `ntfy`. Both send to the same formatter webhook. No Alertmanager route, ntfy authentication or mute was edited.

Dozor's private `~/.local/state/prichal-kvant-public-pds-metrics/` contains the preceding authorized-key/config bytes, full-config validation log and before/installed hashes. The only Prometheus configuration change was insertion of the new rule-file reference. Full configuration validation passed, the existing bind-mount inode was retained, and lifecycle reload succeeded without restarting Prometheus. All ten samples were read back from Prometheus; all four rules were healthy/inactive. The first naturally triggered metrics publication succeeded.

Seven receiver/publisher tests cover status-field exclusion, retained failure history, replay refusal, identity/role/schema/numeric rejection, injection refusal, atomic publication and symlink refusal. Seven promtool cases cover expiry, fresh validity, missed operator heartbeat, missing/stale delivery, failed inspection and failed renewal. The standalone rule harness uses the deployed Prometheus 3.13.1 image's exact OCI digest, a bounded network-disabled disposable container and Docker copy; it removes only its own container. A deliberately invalid fixture returned exit 1, confirming that container failure propagates to CI. These checks are included in hosted CI. All 41 homelab tests and the full pinned application smoke gate passed (44 Node checks, frontend build, Go vet and race-enabled packages). The separate disposable PostgreSQL is stopped with its volume retained. Hosted CI remains a separate result.

## Delivery qualification and recovery

Notification qualification uses one clearly labeled synthetic warning through Alertmanager's existing default route, without changing PDS state, certificate dates or live metrics. The warning route waits two minutes and omits resolved notifications. The initial fixture was cleared after 95 seconds, before the grouping delay; it produced no notification. The dedicated `PrichalKvantPublicPDSCertificateDeliveryTest` group then passed through the same unmuted default warning receiver. Exactly one ntfy cached/published message on the existing `nuc-alerts` topic matched correlation `aefdc0a6-821d-4257-b78b-4d7237d76203`: message `Dxwe4c3PPMZe`, timestamp 1791773230. The fixture was resolved after readback; no resolved notification is sent by that warning receiver. This proves Alertmanager/formatter/ntfy publication, while receipt on a person's device remains unverified.

To roll back, stop/disable only the new publisher timer and stop its service. Remove only the new rule reference after reviewing later config edits, validate and reload in place, then remove the owned rule file/collector symlink if unchanged. Remove only this key's exact restricted authorization line; preserve later keys and other authorizations. Retain private receipts and inspect hashes before restoring config. Preserve Node Exporter, Prometheus, Alertmanager/formatter, ntfy, both earlier PDS services, and pilot recovery/renewal timers.

Naturally due renewal, physical Kvant reboot and refreshed checkpoint restoration remain separate gates. The previous checkpoint predates these monitoring credentials and operator units. No production management provisioning or account identity changed.
