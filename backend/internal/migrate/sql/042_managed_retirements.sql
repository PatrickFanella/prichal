CREATE TABLE managed_endpoint_retirements (
 deployment_id uuid PRIMARY KEY REFERENCES managed_public_endpoints(deployment_id),
 request_key uuid NOT NULL UNIQUE,
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 node_key_hash bytea NOT NULL CHECK(octet_length(node_key_hash)=32),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 native_backup_sha256 bytea NOT NULL CHECK(octet_length(native_backup_sha256)=32),
 management_backup_sha256 bytea NOT NULL CHECK(octet_length(management_backup_sha256)=32),
 phase text NOT NULL DEFAULT 'prepared' CHECK(phase IN ('prepared','released')),
 prepared_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 released_at timestamptz,
 withdrawal_sha256 bytea CHECK(octet_length(withdrawal_sha256)=32),
 native_retirement_sha256 bytea CHECK(octet_length(native_retirement_sha256)=32),
 CHECK((phase='prepared' AND released_at IS NULL AND withdrawal_sha256 IS NULL AND native_retirement_sha256 IS NULL)
    OR (phase='released' AND released_at IS NOT NULL AND withdrawal_sha256 IS NOT NULL AND native_retirement_sha256 IS NOT NULL))
);
