CREATE TABLE enrollment_operations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 recovery_requested boolean NOT NULL DEFAULT false,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','invitation_ready','account_created','verified','expired','failed','superseded')),
 code_ciphertext bytea CHECK(octet_length(code_ciphertext)<=512),
 expires_at timestamptz,
 account_did text,
 account_handle text,
 email_confirmed boolean NOT NULL DEFAULT false,
 message text NOT NULL DEFAULT 'Preparing a first-account invitation.',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 last_refresh_at timestamptz,
 UNIQUE(deployment_id,idempotency_key),
 CHECK((state='invitation_ready' AND code_ciphertext IS NOT NULL AND expires_at IS NOT NULL) OR (state<>'invitation_ready' AND code_ciphertext IS NULL))
);
CREATE INDEX enrollment_operations_deployment ON enrollment_operations(deployment_id,created_at);
CREATE TABLE enrollment_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 enrollment_id uuid NOT NULL REFERENCES enrollment_operations(id),
 state text NOT NULL,
 message text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
