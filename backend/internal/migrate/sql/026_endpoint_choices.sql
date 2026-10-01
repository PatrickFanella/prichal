CREATE TABLE endpoint_choices (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 actor_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key text NOT NULL CHECK(idempotency_key ~ '^[a-zA-Z0-9_-]{1,128}$'),
 hostname text NOT NULL CHECK(length(hostname) BETWEEN 3 AND 244 AND hostname=lower(hostname)),
 application_hosts text[] NOT NULL CHECK(cardinality(application_hosts)<=8),
 state text NOT NULL DEFAULT 'awaiting_ownership' CHECK(state IN ('awaiting_ownership','cancelled')),
 proof_value text NOT NULL CHECK(proof_value ~ '^pdsaas-domain-v1=[a-zA-Z0-9_-]{43}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '7 days',
 cancelled_at timestamptz,
 cancellation_key text CHECK(cancellation_key ~ '^[a-zA-Z0-9_-]{1,128}$'),
 UNIQUE(organization_id,idempotency_key),
 CHECK((state='cancelled' AND cancelled_at IS NOT NULL AND cancellation_key IS NOT NULL) OR (state='awaiting_ownership' AND cancelled_at IS NULL AND cancellation_key IS NULL))
);
-- An unverified hostname is never a global domain reservation.
CREATE UNIQUE INDEX endpoint_choices_pending_organization ON endpoint_choices(organization_id) WHERE state='awaiting_ownership';
CREATE TABLE endpoint_choice_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 choice_id uuid NOT NULL REFERENCES endpoint_choices(id),
 actor_id uuid NOT NULL REFERENCES owners(id),
 action text NOT NULL CHECK(action IN ('save','cancel')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
