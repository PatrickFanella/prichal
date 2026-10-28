CREATE TABLE managed_native_conversions (
 deployment_id uuid PRIMARY KEY REFERENCES managed_public_endpoints(deployment_id),
 request_key uuid NOT NULL UNIQUE,
 node_id uuid NOT NULL REFERENCES hosting_nodes(id),
 node_key_hash bytea NOT NULL CHECK(octet_length(node_key_hash)=32),
 previous_hostname text NOT NULL CHECK(previous_hostname=deployment_id::text||'.localhost'),
 previous_endpoint text NOT NULL,
 native_backup_sha256 bytea NOT NULL CHECK(octet_length(native_backup_sha256)=32),
 management_backup_sha256 bytea NOT NULL CHECK(octet_length(management_backup_sha256)=32),
 operator_receipt_sha256 bytea NOT NULL CHECK(octet_length(operator_receipt_sha256)=32),
 credentials_sha256 bytea NOT NULL CHECK(octet_length(credentials_sha256)=32),
 certificate_sha256 bytea NOT NULL CHECK(octet_length(certificate_sha256)=32),
 native_receipt_sha256 bytea NOT NULL CHECK(octet_length(native_receipt_sha256)=32),
 actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 128),
 confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
