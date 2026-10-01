CREATE TABLE hosting_nodes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 region text NOT NULL,
 agent_endpoint text NOT NULL UNIQUE,
 slots integer NOT NULL CHECK(slots BETWEEN 1 AND 1000),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','active','draining','disabled')),
 public_key_hash bytea,
 certificate_pem bytea,
 enrolled_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE node_bootstrap_tokens (
 node_id uuid PRIMARY KEY REFERENCES hosting_nodes(id),
 token_hash bytea NOT NULL UNIQUE,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz
);
CREATE TABLE instance_placements (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id),
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX instance_placements_node ON instance_placements(node_id);
