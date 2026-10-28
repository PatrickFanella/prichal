CREATE TABLE managed_public_endpoints (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id),
 hostname text NOT NULL UNIQUE,
 forward_port integer NOT NULL UNIQUE CHECK(forward_port BETWEEN 35000 AND 35999),
 reservation_basis text NOT NULL CHECK(reservation_basis IN ('queued','empty_fixture')),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 reserved_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(deployment_id::text ~ '^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$'),
 CHECK(hostname='pds-' || replace(deployment_id::text,'-','') || '.subcult.tv')
);
CREATE TABLE managed_edge_observations (
 sequence bigserial PRIMARY KEY,
 deployment_id uuid NOT NULL REFERENCES managed_public_endpoints(deployment_id),
 request_key uuid NOT NULL,
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 node_key_hash bytea NOT NULL CHECK(octet_length(node_key_hash)=32),
 certificate_pem bytea NOT NULL CHECK(octet_length(certificate_pem) BETWEEN 1 AND 16384),
 certificate_sha256 bytea NOT NULL CHECK(octet_length(certificate_sha256)=32),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(deployment_id,request_key)
);
CREATE INDEX managed_edge_observations_latest ON managed_edge_observations(deployment_id,sequence DESC);
