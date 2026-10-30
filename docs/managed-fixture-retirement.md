# Staging-02 retirement preparation — October 30, 2026

The owner authorized retiring the empty staging-02 fixture after preserving its backup, then reusing that node for #7's fresh API-to-native allocation drill. This uses the existing 3-vCPU/4-GiB/40-GiB disposable Kvant VM and incurs no provider spend. The first staging node and other pilots remain outside this operation.

## Preservation result

Fresh preflight confirmed the four retained native/ingress/predecessor IDs, unchanged credentials, zero native accounts, no enrollment operations, one ready deployment/placement/endpoint, no unfinished management work and no recovery hold. PR #215 merged as `169e2d6c167394c82509707b24d641f194ffa27f` after required exact-head CI passed, without comments/reviews or conflict; its watch is stopped.

The dedicated schema-41 management snapshot restored all 69 public tables and every sequence with matching logical contents. The source stayed unchanged and the exact allocator-owned disposable restore database was dropped. Its retained SHA256 is `bdb1f5548f47ee972a0386cdce0c879758b723dcdb4b1d3c91597ed56bcfff66`.

The agent was stopped before acquiring its exclusive native-state lock. The fixed operator then stopped only the two managed containers, preserving stopped private predecessors, and archived native data/blobs, retained native credentials/TLS/configuration and enrolled node key/certificates. Public CA, installed agent binary, unit configuration and private container metadata were retained alongside the archive. Both source and restored inventories matched all ten regular files; WAL-aware SQLite integrity and zero-account checks passed. Archive SHA256 is `1b6617c6068fc8dfaa45e0c8cda34f9a09293684e63c89e6c91c41301fe4a56f`.

The first attempt correctly refused completion when a SQLite integrity read changed WAL/SHM cache files in the restored tree. The source was unchanged and stayed cold. The corrected procedure preserves the exact comparison restore and performs SQLite checks on a separate copy. An explicit stopped-fixture continuation used a new request key and retained the failed attempt. A real WAL regression covers this behavior. Neither attempt deleted a container, volume, management row, credential or route.

The verified archive and adjacent recovery inputs were copied out of the guest into private Kvant custody. The copied archive digest matched; independent extraction matched all ten file hashes and another disposable SQLite verification passed. This provides guest-local and off-guest copies on the same physical host. It does not prove survival of Kvant loss, a full VM boot restore or application recovery after later managed writes.

The original agent restarted, then the existing pinned-mTLS `EnsureManaged` qualification path resumed the original containers. Final native readback confirmed healthy resources, one writer, zero accounts, unchanged credentials and stopped private predecessors. Fresh public TLS/PDS/OAuth/signup-closure/unknown-host/WebSocket checks passed. Management remains ready with one placement, four historical public checks, one retry and no unfinished jobs; no new durable public proof was committed. Node transport expiry remains October 30 at 22:18:46 UTC and needs fresh verification before subsequent live work.

## Operator contract and custody

`deploy/homelab/preserve-staging-02-managed.py` is a guest-root preservation tool for only the fixed staging-02 fixture. It requires the expected guest hostname, explicit `--confirm-empty-staging-02`, a canonical new `--request-key`, inactive agent and the existing exclusive state lock. Exact container IDs, generated runtime identity, closed registration, storage bindings and disabled restart must match. It checks zero accounts before stopping a running fixture. A cold continuation additionally requires `--confirm-stopped-fixture` and proves zero accounts from its WAL-aware copied data. Mixed running/stopped managed resources refuse.

Private output is exclusive and bounded. Symlinks, special files, escaped archive members, changed source/restore contents, populated SQLite, existing request directories and excess backup size refuse. Failures leave the agent/native state stopped for inspection; the tool does not resume, retire, withdraw routing or release capacity. Recoverability must be verified before any later destructive step. It is not an API/worker command.

Guest-root custody is `/var/lib/prichal-agent/managed-retirements/c3b6c1be-8493-43f4-9a79-ad6f3cbab521/`. Kvant operator custody is `~/.local/state/prichal-staging-02-bootstrap/managed-retirement-20261030/`; `native-v2/` holds the copied archive, recovery inputs and independent restore. The preceding failed guest request is `ec26439d-45c5-449c-b445-26807191d2f4`. Corrected source SHA256 is `f28c1e06d2b2776f61a90e6eb985fbc7fd0b2a830c5e7369fe2088f1db265802`; root helpers retain distinct original/v2 paths under `/usr/local/libexec/prichal/`. The installed agent binary and its keys were not replaced.

Five preservation checks passed, including real SQLite/WAL archive recovery, populated-fixture refusal before stop, changed runtime/predecessor refusal, unsafe archive inputs and exclusive output. Full pinned smoke and final repository/canonical checks are recorded in the progress log. Private runtime receipts and logs remain in ignored local storage and the custody paths above.

## Remaining retirement and fresh allocation

The fixture is preserved and running; it is **not retired and its slot is not free**. Its public route, forwards, node identity, placement and management history remain intact. The authorized next steps are guarded withdrawal of its exact endpoint, a durable retirement record that clears serving state while retaining identity/history, fencing stale reconciliation, and releasing only its native/placement capacity. Preserve the verified archives and original volumes. Do not roll back the entire management registry or discard unrelated state to obtain a slot.

The current publisher deliberately refuses endpoint omission/remapping. Retirement needs an explicit withdrawal path with current owned/unrelated-source checks, full Caddy validation, guarded reload/readback and a retained receipt; ordinary publication/rollback must not resurrect a retired endpoint. Fresh allocation then needs a new API-created deployment identity, restricted native reconciliation, an operator handoff based on the fresh export and successful durable public verification. Keep enrollment closed for #8. This preparation does not qualify those remaining steps or close #7.

The subsequent [schema-42 management retirement record](managed-retirement-record.md) implements separate preparation and receipt/namespace-fenced placement release. Its fixed operator command adds no host authority. It has not been applied to the live registry or substituted for the remaining publisher/native steps.

[Withdrawal and native retirement](managed-fixture-withdrawal.md) adds the fixed publisher withdrawal and guest hold/removal tools that produce the release receipts.
