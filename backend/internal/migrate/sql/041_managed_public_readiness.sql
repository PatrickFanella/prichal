CREATE TABLE managed_public_readiness (
 deployment_id uuid PRIMARY KEY REFERENCES managed_public_endpoints(deployment_id),
 cycle integer NOT NULL DEFAULT 1 CHECK(cycle>0),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 3),
 next_check_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE managed_public_checks (
 sequence bigserial PRIMARY KEY,
 deployment_id uuid NOT NULL REFERENCES managed_public_readiness(deployment_id),
 cycle integer NOT NULL CHECK(cycle>0),
 attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 3),
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 node_key_hash bytea NOT NULL CHECK(octet_length(node_key_hash)=32),
 hostname text NOT NULL CHECK(hostname='pds-'||replace(deployment_id::text,'-','')||'.subcult.tv'),
 code text NOT NULL CHECK(code IN ('ready','dns','tls','health','identity','websocket','unknown_host','registration','native')),
 addresses text[] NOT NULL DEFAULT '{}' CHECK(cardinality(addresses)<=16),
 certificate_sha256 bytea CHECK(certificate_sha256 IS NULL OR octet_length(certificate_sha256)=32),
 certificate_expires_at timestamptz,
 native_version text NOT NULL DEFAULT '' CHECK(length(native_version)<=64),
 unknown_hostname text NOT NULL DEFAULT '' CHECK(length(unknown_hostname)<=253),
 observed_at timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(deployment_id,cycle,attempt),
 CHECK(code<>'ready' OR (cardinality(addresses)>0 AND certificate_sha256 IS NOT NULL AND certificate_expires_at IS NOT NULL AND certificate_expires_at>observed_at AND native_version<>'' AND unknown_hostname<>''))
);
CREATE TABLE managed_public_retries (
 deployment_id uuid NOT NULL REFERENCES managed_public_readiness(deployment_id),
 request_key uuid NOT NULL,
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 cycle integer NOT NULL CHECK(cycle>1),
 requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(deployment_id,request_key),
 UNIQUE(deployment_id,cycle)
);
