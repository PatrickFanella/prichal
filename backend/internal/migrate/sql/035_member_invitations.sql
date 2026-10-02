-- Member invitations let an owner admit additional native accounts on their PDS.
-- Codes are encrypted transient values; accepted rows record only public identity.
CREATE TABLE member_invitations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','invitation_ready','accepted','expired','withdrawing','withdrawn','ceiling_reached','owner_required','failed')),
 code_ciphertext bytea CHECK(octet_length(code_ciphertext)<=512),
 expires_at timestamptz,
 account_did text CHECK(account_did ~ '^did:plc:[a-z2-7]{24}$'),
 account_handle text CHECK(char_length(account_handle) BETWEEN 1 AND 253),
 message text NOT NULL,
 withdrawal_requested_at timestamptz,
 last_refresh_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(deployment_id,idempotency_key),
 CONSTRAINT member_invitation_code CHECK((state='invitation_ready' AND code_ciphertext IS NOT NULL AND expires_at IS NOT NULL) OR (state<>'invitation_ready' AND code_ciphertext IS NULL)),
 CONSTRAINT member_invitation_account CHECK((state='accepted')=(account_did IS NOT NULL AND account_handle IS NOT NULL)),
 CONSTRAINT member_invitation_withdrawal CHECK(state NOT IN ('withdrawing','withdrawn') OR withdrawal_requested_at IS NOT NULL)
);
CREATE INDEX member_invitations_deployment ON member_invitations(deployment_id,created_at);
CREATE UNIQUE INDEX member_invitations_account ON member_invitations(deployment_id,account_did) WHERE account_did IS NOT NULL;
CREATE TABLE member_invitation_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 invitation_id uuid NOT NULL REFERENCES member_invitations(id),
 state text NOT NULL,
 message text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
