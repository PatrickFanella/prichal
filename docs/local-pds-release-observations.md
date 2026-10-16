# PDS release observations

Issue #154 adds a read-only release view. Owners see their own deployments' observations; operators see the same fields in an audited instance inspection and a separate latest-upstream lookup. Upgrade qualification and execution remain under #15.

The work follows three boundaries: inspect the running container in the restricted node agent; record the result through the existing authenticated monitor in management PostgreSQL; project it through the existing owner/operator authorization and display it in the corresponding screen. The management API receives no Docker socket, host credentials or native-account session.

## Runtime evidence

`Docker.Status` first applies the existing namespace, image-pin, environment, ingress and network validation. For a running native container, it reads `container.Config.Image` and `container.Image`, then inspects that immutable image ID. It requires the image inspection's ID to match and `RepoDigests` to contain the manifest digest in the container's reference. Missing or inconsistent evidence yields no release metadata. It never substitutes the compile-time pin as an observed version.

The version comes from `org.opencontainers.image.version` in that inspected image's OCI labels. A missing or malformed version leaves the exact version unknown while retaining a valid observed tag/digest. The container image ID and manifest/index digest have different meanings, even when their values happen to match. The owner view shows the version and twelve hexadecimal digest characters; the management projection retains the full reference, digest and image ID. It excludes container environment, host paths and labels unrelated to the version.

The monitor records a sample for each inspected ready placement, including an unknown sample when inspection fails or returns another instance. Writes and reads require the current placement's node ID. Failed whole scans do not commit partial results. Observations older than one minute are marked stale; stale values describe the last observation rather than claiming the process is currently running. Unplaced or non-ready deployments show unknown.

Schema 36 adds `instance_runtime_releases`, separate from every PDS's SQLite databases and blobs. Only the monitor writes these rows. The existing owner projection checks organization ownership before reading them; the operator projection commits its normal inspection audit before returning them. The owner browser also rejects foreign-organization responses, clears the list on unavailable/denied reads, and ignores superseded polling responses.

## Upstream lookup

`GET /api/operator/pds-upstream-release` uses the existing operator browser-session gate, including refusal of bearer credentials. It accepts no target URL and sends no management or native credentials upstream. It reads only the fixed [official latest-release endpoint](https://api.github.com/repos/bluesky-social/pds/releases/latest), with a three-second deadline, a 64 KiB response limit, no redirects and a validated non-draft/non-prerelease version tag.

One process-wide cache admits one in-flight lookup. Page requests return immediately while lookup proceeds independently. Success is cached for one hour; failure for five minutes. Expired entries and failed lookups display unknown, with the last completed check time when available. An API restart starts with unknown. Upstream failure never blocks deployment inspection or other pages. The UI polls the local cached endpoint, not GitHub.

On October 16, 2026, the official endpoint returned HTTP 404 and the [upstream releases page](https://github.com/bluesky-social/pds/releases) showed no published releases. Unknown is the expected live result. Tests and browser checks use an explicitly synthetic positive release. Repository commits, image tags and a new upstream release are not automatically qualified upgrade targets; no comparison badge or upgrade control claims otherwise.

## Qualification and rollout

The pinned local gate `./scripts/dev.sh smoke` passed 44 Node checks, frontend production build, Go vet and all Go race packages using the separate qualification PostgreSQL. Tests cover valid and missing/mismatched image evidence, a different inspected tag, stopped runtime, unavailable engine, monitor persistence and failure clearing, placement mismatch, owner denial, stale samples, historical migrations, cache reuse/concurrency and unreachable, absent, oversized, malformed or unsafe upstream responses. A first full run caught two test fixtures missing the required `operator_slots`; those fixtures were corrected before the passing full run.

An optional native check reads an explicitly selected `prichal-release-<UUID>` container through the Docker Engine API. It refuses an unowned name/label, network access, mounts, published ports or a different image. Run only against a fresh isolated fixture:

```sh
PDS_TEST_RELEASE_CONTAINER=prichal-release-<UUID> go test -race -count=1 -v ./internal/nodeagent -run '^TestNativeDockerReleaseObservation$'
```

The successful native fixture `d89e737f-ffec-42e0-954f-5e8bc0f1b441` observed image version `0.4.5036` and the pinned manifest digest through the new observer. It ran only an idle Node process from the official image, with network `none`, no mounts/ports, a read-only root, resource caps and dropped capabilities. This proves native Engine image inspection, not a new PDS account journey. The fixture stopped successfully and remains retained. Private evidence is under `.local/pds-release/prichal-release-d89e737f-ffec-42e0-954f-5e8bc0f1b441/`; the initial private-directory test setup failure remains separate and stopped.

Thirteen controlled browser states covered owner observed/stale/unknown/malformed/missing-version values, authorization loss/recovery, foreign organization rejection, late success after denial, operator inspection/denial, upstream absence and failed upstream API reads. No mutation was submitted and owners made no upstream-release request. Widths 320, 667 and 1280 had no horizontal overflow. The fixture renders the actual owner/operator components with synthetic API responses; it does not prove deployed management integration. Browser evidence is retained privately in `.local/pds-release-browser.json`.

Roll out compatible management API, workers and monitor with schema 36 before upgrading node agents: older agents omit the new optional metadata and display unknown, while older workers reject unknown fields from newer agents. The additive table does not change PDS data or runtime policy. Existing schema-version guards refuse old binaries after migration; rollback requires a reviewed management database checkpoint and coordinated binary rollback, or a forward fix. Do not lower the schema marker or drop retained observations to bypass that guard. Stopping the new monitor leaves observations to become stale. The separate qualification PostgreSQL had zero generated smoke/recovery databases remaining and was stopped; its volume and both stopped image fixtures remain retained. The Vite fixture preview was stopped and its browser timers removed by navigation. Hosted CI, merge and production deployment remain separate proof states.
