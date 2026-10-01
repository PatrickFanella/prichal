CREATE TABLE recovery_retained_namespaces (
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 instance_id uuid NOT NULL,
 reason text NOT NULL CHECK(reason='local-qualification-fixture'),
 acknowledged_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(node_id,instance_id)
);
