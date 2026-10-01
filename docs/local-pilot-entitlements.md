# Local pilot entitlement qualification

New native deployment intents require an explicit active hosting exception. This slice implements local operator pilot grants and native instance allowance enforcement, not paid checkout or a published pricing offer. Existing organizations receive no implicit exception. Simulated deployments remain non-billable local qualification work.

The immutable local profiles are `local-single` (one native deployment intent) and `local-small` (three). A grant lasts 1 to 720 hours. Prices, storage allowances, paid grace rules and the Stripe sandbox are not configured. An operator records a reason (`pilot` or `qualification`), a stable request UUID and their operating-system identity. Repeat requests return the original audit record without extending expiry; changed payload or actor returns a conflict. Replaying an older grant does not replace a newer revoke or downgrade.

```sh
./scripts/dev.sh entitlement-admin grant --organization ORGANIZATION_UUID \
  --key REQUEST_UUID --plan local-single --hours 24 --reason qualification
./scripts/dev.sh entitlement-admin revoke --organization ORGANIZATION_UUID \
  --key NEW_REQUEST_UUID --reason qualification
```

Replace placeholders with actual UUIDs. These commands require explicit local mode and the loopback `pds_local` database. No customer or CLI endpoint can mutate grants. The owner reads only their organization's safe status at `GET /api/organizations/{id}/entitlement` and sees it beside deployments.

The deployment transaction first locks the organization and resolves stable-key replay. For a new native intent it checks the latest grant, expiry and retained native allocation before inserting the deployment, event and durable job. Grant/revoke/downgrade operations take the same organization lock, so concurrent allocation cannot exceed the allowance. Failed or uncertain requests continue to count until an explicit release procedure is qualified. Allowance eligibility does not promise fleet capacity; the separate node reservation/host capacity checks still apply.

Expiry, revocation or downgrade restricts new allocation. It does not cancel accepted work, stop servers, remove data, alter native account authority or prevent status/replay. There is no automatic overage charge, deletion, storage quota, customer export or resource-release operation in this slice. Account enrollment retains its independently qualified first-account fence. Usage measurements continue to report that total storage limits are unconfigured.

## Evidence

The full smoke gate passed with frontend build, Go vet and race-enabled clean-database integration tests. Tests covered denial before job insertion, grant replay/expiry, actor/payload conflicts, owner isolation, eight concurrent native submissions yielding exactly one accepted allocation, retained failed allocation, upgrade to three, downgrade to one without deleting three allocations/jobs, revocation and accepted-deployment replay.

Actual local qualification `cdc543e2-d2cb-4f36-bd9f-5e02df2f46fe` exercised schema 18 against disposable Alice/Bob organizations. An unconfigured request returned 403; a one-instance allowance over Alice's two retained native intents returned 409. A three-instance grant reported one remaining entitlement allocation, then downgrade blocked growth while retaining both intents. Revocation returned 403 for new work. Bob's stable native request still returned the same ready placement while revoked. Foreign-owner status returned 404 and a customer grant mutation returned 405. Source deployment/event/deploy-job counts stayed unchanged. Both tenants retained container identities and credential/TLS/admission digests.

The drill left explicit, audited 24-hour local qualification grants: Alice `local-small`, Bob `local-single`. The node still has three occupied slots; no new native host action was attempted. No payments or Stripe resources were created. Report: `.local/pilot-entitlement/cdc543e2-d2cb-4f36-bd9f-5e02df2f46fe/report.json`.

Management recovery `cc1378d3-81af-4b95-94e5-88d2ace176cc` matched all 30 schema 18 management/River tables and sequence positions, including all eight grant audit records. The restored hold revoked copied authentication and refused worker startup. Host reconciliation and lost-checkpoint replay preserved source rows/sequences, native signed data/media and protected host state. The latest grant states/expiries and derived native allocation counts matched the source for both organizations. Reports remain in `.local/management-recovery/cc1378d3-81af-4b95-94e5-88d2ace176cc/`. Source management services resumed afterward.

## Paid integration and remaining gates

A paid implementation must use hosted subscription Checkout and Customer Portal in an isolated Stripe sandbox, verify raw-body webhook signatures, deduplicate events durably and reconcile authoritative subscription state before granting entitlement. A Checkout success page cannot authorize deployment. Payment failures must follow an explicit grace policy while retaining data. Product/Price mappings, restricted API keys, webhook delivery and required tax registrations/checkout tax behavior must be qualified before live charges. See [subscription webhook documentation](https://docs.stripe.com/billing/subscriptions/webhooks) and [webhook verification documentation](https://docs.stripe.com/webhooks).

Storage/account limits, paid plan configuration, billing event ordering, actual checkout/portal flows, resource release, notification delivery and live qualification remain open. Responsive browser checks remain unavailable in the collaborative preview. Issues #18 and #19 remain open.
