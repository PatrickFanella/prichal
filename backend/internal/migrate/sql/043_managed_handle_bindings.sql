CREATE TABLE managed_handle_bindings (
 invitation_id uuid PRIMARY KEY,
 deployment_id uuid NOT NULL REFERENCES managed_public_endpoints(deployment_id),
 hostname text NOT NULL UNIQUE CHECK(hostname ~ '^at-[0-9a-f]{15}\.subcult\.tv$'),
 did text NOT NULL UNIQUE CHECK(did ~ '^did:plc:[a-z2-7]{24}$'),
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 node_key_hash bytea NOT NULL CHECK(octet_length(node_key_hash)=32),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 verified_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX managed_handle_bindings_deployment ON managed_handle_bindings(deployment_id);
