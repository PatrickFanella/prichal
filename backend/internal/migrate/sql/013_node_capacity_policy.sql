CREATE TABLE hosting_node_policy_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 reason text NOT NULL CHECK(reason IN ('local_isolation_qualification','planned_capacity_change','capacity_reduction')),
 previous_slots integer NOT NULL,
 requested_slots integer NOT NULL CHECK(requested_slots BETWEEN 1 AND 8),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX hosting_node_policy_events_node ON hosting_node_policy_events(node_id,id DESC);
