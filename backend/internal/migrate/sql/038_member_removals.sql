-- Accepted invitations and native identities remain retained after member exit.
CREATE TABLE member_removals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 invitation_id uuid NOT NULL UNIQUE REFERENCES member_invitations(id),
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 requested_by uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 account_did text NOT NULL CHECK(account_did ~ '^did:plc:[a-z2-7]{24}$'),
 state text NOT NULL DEFAULT 'requested' CHECK(state IN ('requested','migration_window','deactivated','review')),
 message text NOT NULL,
 host_requested_at timestamptz,
 deactivate_after timestamptz,
 completed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(deployment_id,idempotency_key),
 CHECK((host_requested_at IS NULL)=(deactivate_after IS NULL)),
 CHECK(deactivate_after IS NULL OR extract(epoch FROM(deactivate_after-host_requested_at))=2592000),
 CHECK(state NOT IN ('migration_window','deactivated') OR deactivate_after IS NOT NULL),
 CHECK((state='deactivated')=(completed_at IS NOT NULL))
);
CREATE TABLE member_removal_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 removal_id uuid NOT NULL REFERENCES member_removals(id),
 state text NOT NULL,
 message text NOT NULL,
 actor_id uuid REFERENCES owners(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX member_removal_events_history ON member_removal_events(removal_id,id);
