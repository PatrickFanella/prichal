ALTER TABLE service_observations DROP CONSTRAINT service_observations_kind_check;
ALTER TABLE service_observations ADD CONSTRAINT service_observations_kind_check CHECK(kind IN ('host_unavailable','runtime_unhealthy','capacity_exhausted','job_backlog','deployment_failed','recovery_held','backup_unconfigured','usage_unavailable','disk_pressure','account_ceiling_exceeded'));
CREATE TABLE instance_usage (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id),
 data_bytes bigint NOT NULL CHECK(data_bytes>=0),
 blob_bytes bigint NOT NULL CHECK(blob_bytes>=0),
 data_files bigint NOT NULL CHECK(data_files>=0),
 blob_files bigint NOT NULL CHECK(blob_files>=0),
 accounts bigint NOT NULL CHECK(accounts>=0),
 measured_at timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
