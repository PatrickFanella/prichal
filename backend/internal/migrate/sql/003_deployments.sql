CREATE TABLE deployments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 63),
 region text NOT NULL,
 idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 128),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','configuring','checking','ready','failed')),
 message text NOT NULL DEFAULT 'Waiting for a worker.',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,idempotency_key),
 UNIQUE(organization_id,name)
);
CREATE TABLE deployment_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 state text NOT NULL,
 message text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX deployment_events_operation ON deployment_events(deployment_id,id);
CREATE TABLE test_instances (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
