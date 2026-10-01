# PDS-11: Protect credentials and prove identity-key recovery

[Gitea issue #11](https://git.subcult.tv/PatrickFanella/pds-ass/issues/11)

<!-- pds-ass-roadmap:11 -->

## What to build

An operator can securely provision runtime credentials and recover a disposable account's identity from independently protected recovery material.

## Acceptance criteria

- [ ] Choose the supported runtime encryption/KMS and recovery-custody approach before paid customer use.
- [ ] Separate tenant credentials, provider tokens, PLC rotation material and recovery-key access.
- [ ] Store references rather than secret values in jobs, logs, browser responses, exports and audit records.
- [ ] Rehearse key recovery and credential rotation with protected backups; identify unrecoverable conditions honestly.
- [ ] Capture administrative access/rotation evidence without revealing key material.

## Verification

Scan artifacts and logs for leaked values, rotate designated staging credentials and recover identity with the documented recovery path.

## Blocked by

- #3: Bootstrap and register a staging hosting node
- #4: Qualify a disposable official PDS runtime

## Boundaries

SOPS configuration encryption alone is not a full identity recovery plan; customer-held keys are opt-in and require a supported custody flow.

## Planning reference

Phase: M2 - Recoverable managed service. Priority: P0.
[Implementation plan](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/implementation-plan.md) and [roadmap](https://git.subcult.tv/PatrickFanella/pds-ass/src/branch/main/docs/roadmap.md).
