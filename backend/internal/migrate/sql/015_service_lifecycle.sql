CREATE TABLE service_operations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 idempotency_key uuid NOT NULL,
 generation bigint NOT NULL CHECK(generation>0 AND generation<=9007199254740992),
 desired_state text NOT NULL CHECK(desired_state IN ('running','suspended')),
 prior_state text NOT NULL CHECK(prior_state IN ('running','suspended','unknown')),
 reason text NOT NULL CHECK(reason IN ('maintenance','safety_review','resource_pressure')),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','applied','failed')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(deployment_id,idempotency_key),
 UNIQUE(deployment_id,generation)
);
CREATE TABLE service_operation_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 operation_id uuid NOT NULL REFERENCES service_operations(id),
 state text NOT NULL CHECK(state IN ('pending','applied','failed')),
 created_at timestamptz NOT NULL DEFAULT now()
);
