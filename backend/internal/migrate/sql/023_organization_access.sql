CREATE TABLE organization_members (
 organization_id uuid NOT NULL REFERENCES organizations(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 role text NOT NULL CHECK(role IN ('administrator','viewer')),
 state text NOT NULL CHECK(state IN ('active','revoked')),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,owner_id)
);
CREATE TABLE organization_invitations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 inviter_id uuid NOT NULL REFERENCES owners(id),
 target_id uuid NOT NULL REFERENCES owners(id),
 target_issuer text NOT NULL,
 target_subject text NOT NULL,
 role text NOT NULL CHECK(role IN ('administrator','viewer')),
 idempotency_key uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','accepted','expired','revoked')),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours',
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,idempotency_key),
 CHECK(inviter_id<>target_id)
);
CREATE UNIQUE INDEX organization_invitations_pending ON organization_invitations(organization_id,target_id) WHERE state='pending';
CREATE TABLE organization_transfers (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','accepted','expired','revoked')),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours',
 previous_owner_id uuid NOT NULL REFERENCES owners(id),
 next_owner_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,idempotency_key),
 CHECK(previous_owner_id<>next_owner_id)
);
CREATE UNIQUE INDEX organization_transfers_pending ON organization_transfers(organization_id) WHERE state='pending';
CREATE TABLE organization_access_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES organizations(id),
 actor_id uuid NOT NULL REFERENCES owners(id),
 target_id uuid REFERENCES owners(id),
 action text NOT NULL CHECK(action IN ('invite','accept','expire','revoke','role','rename','transfer_offer','transfer_accept')),
 role text CHECK(role IN ('administrator','viewer')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX organization_access_events_history ON organization_access_events(organization_id,id DESC);
