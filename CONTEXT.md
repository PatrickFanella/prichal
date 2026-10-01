# Managed PDS hosting

The service provides independently managed hosting for creators and organizations. Hosting management and native account authority are separate.

## Language

**Management owner**:
The authenticated person responsible for an organization and its hosting decisions.
_Avoid_: PDS account owner, billing customer

**Native account**:
An AT identity's account, signed repository and private settings held by a PDS.
_Avoid_: Management login, organization

**Hosting exit**:
An owner's dated request to end a particular hosting service while preserving a supported opportunity to recover or migrate its data.
_Avoid_: Account deletion, payment cancellation

**Exit reversal**:
Withdrawal of a hosting exit within its supported recovery window. Reversal does not revoke independent operator holds or account moderation.
_Avoid_: Moderation appeal, payment reinstatement

**Retention window**:
The period in which ended hosting data remains retained for supported recovery or migration.
_Avoid_: Export download window, deletion confirmation

**Retention review**:
A review due after the stated retention period. It is not authority to delete identity or retained data.
_Avoid_: Automatic deletion, account erasure

**Payment cancellation**:
A change to subscription billing with its own authority, effective date and financial consequences.
_Avoid_: Hosting exit, data deletion

**Organization member**:
A verified management identity with a current, scoped role in a hosting organization.
_Avoid_: Native account delegate, shared login

**Management invitation**:
An expiring offer of a scoped organization role to one exact verified management identity.
_Avoid_: Email identity, enrollment invitation

**Ownership handover**:
An explicit transfer of organization management responsibility that preserves a single owner and requires separate native custody decisions for populated hosting.
_Avoid_: Account migration, signing delegation

**Deployment admission**:
The operator policy that accepts or pauses new deployment intent while preserving committed work, existing service and supported recovery.
_Avoid_: Service suspension, account registration, public launch approval
