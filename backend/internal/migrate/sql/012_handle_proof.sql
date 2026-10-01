CREATE TABLE handle_configurations (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 handle text NOT NULL CHECK(length(handle) BETWEEN 3 AND 244),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision > 0),
 state text NOT NULL CHECK(state IN ('pending_dns','checking','pending_identity','verified','conflict','unavailable','invalid')),
 message text NOT NULL,
 checked_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE handle_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 handle text NOT NULL,
 state text NOT NULL,
 message text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX handle_events_deployment ON handle_events(deployment_id,id DESC);
