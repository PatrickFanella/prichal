# Local hosting exit rehearsal

An authenticated organization owner can choose a verified, unexpired portable export, confirm they saved a copy and review a dated service end and retention period. The request binds the exact policy shown. Stable requests preserve their original dates across retries and configuration changes. See [the exit contract](local-hosting-exit-design.md) for authority and retention semantics.

## Local configuration

New scheduling is disabled by default. `PDS_LOCAL_EXIT_POLICY_FILE` can point to a private, regular, nonsymlink file under ignored `.local/`. It accepts only `version: 1`, `delaySeconds` between 30 and 604800 and `retentionDays` between 1 and 30. Its permissions must exclude group and other access; use 0600. Production configuration rejects this rehearsal policy. Existing committed requests and supported reversal remain available after disabling the policy.

Scheduling commits the exit, audit and two River jobs together. The worker creates a fixed service-stop fence through the restricted native agent. Retention expiry records support review; it grants no deletion authority. Stopping preserves containers, volumes, identity custody and independent account moderation. Reversal can resume only that exit's exact applied service stop within its retention window. A newer operator fence, pending or uncertain outcome requires review.

Owner routes require browser authentication, organization ownership and CSRF for mutations. CLI credentials cannot access these routes. The owner interface stores an intent before submission and requires explicit review before retry. Payment cancellation, automatic deletion and live retention are not enabled.

## Qualification

Normal drill `49ff3507-742e-4445-8c0c-58c1f1170222` completed at 2026-10-01T14:05:44Z. Withdrawal before service end caused no host operation. A second exit stopped Alice's native service; the retained archive still downloaded with its original hash. Reversal resumed that service. During the exercise 144 authenticated Bob reads succeeded. Signed records, referenced media, container identities and protected credentials/TLS/admission state remained unchanged. Foreign-owner requests, missing CSRF, unsaved-copy confirmation and changed stable payloads were denied.

Held-account drill `86bf94a1-6283-400d-b112-83aff66b915f` completed at 2026-10-01T14:20:33Z. Alice's generation-7 account hold remained applied while exit service generations 8 and 9 stopped and resumed hosting. Public native login remained denied after owner reversal. A separate operator account action at generation 10 restored login. All signed data, media and protected custody were retained. An earlier wrapper lost a pooled TLS connection after a service restart; its cleanup completed. Local native checks now use a fresh TLS connection, and the complete repeated drill passed. Failed audit evidence remains retained.

Policy drill `4826886d-b3c4-41c1-9fd6-5bf837294544` completed at 2026-10-01T14:33:00Z. Mismatched reviewed policy and changed stable payloads were rejected. Exact retries retained original dates. Early reversal created no native transition. With policy disabled again, actual owner requests replayed the committed schedule/reversal and a new schedule was refused at 2026-10-01T14:56:08Z.

Schema-22 management recovery `cccc7a4b-7394-41eb-8424-e30d0817b654` matched all 39 tables and sequence positions, including seven exits, 23 exit audits, seven reversals, 36 lifecycle operations, 72 lifecycle audits, four export operations and 79 River jobs. Copied authentication was revoked and held worker startup refused. Reconciliation and lost-checkpoint replay preserved source rows/sequences, native signed data/media and protected lifecycle/admission custody. The replay probe uses a dedicated queue. A copied retention job was made available in the disposable database; it and every copied default-queue row remained unchanged by the probe.

The full local smoke gate passed frontend build, Node checks and Go vet/race/PostgreSQL tests. Coverage includes atomic second-job failure rollback, concurrent scheduling, ownership, stable replay, recovery holds, independent operator fences, retention review without cleanup and failure review. Browser rendering, accessibility and persistence remain unqualified after the existing collaborative preview snapshot failure. Optional golangci-lint is unavailable.

All seven source exits are reversed. Both native services are running. New scheduling, fixture operator authority and billing are disabled. No retained recovery copy, container or volume was removed. Commercial cancellation, independent custody, notifications, confirmed disposable deletion, migration and live provider qualification remain open.
