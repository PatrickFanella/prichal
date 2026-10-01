CREATE TABLE service_observations (
 subject_key text NOT NULL CHECK(length(subject_key) BETWEEN 1 AND 120),
 deployment_id uuid REFERENCES deployments(id),
 kind text NOT NULL CHECK(kind IN ('host_unavailable','runtime_unhealthy','capacity_exhausted','job_backlog','deployment_failed','recovery_held','backup_unconfigured')),
 active boolean NOT NULL,
 first_observed_at timestamptz NOT NULL DEFAULT now(),
 last_observed_at timestamptz NOT NULL DEFAULT now(),
 resolved_at timestamptz,
 PRIMARY KEY(subject_key,kind),
 CHECK((active AND resolved_at IS NULL) OR (NOT active AND resolved_at IS NOT NULL))
);
CREATE INDEX service_observations_deployment ON service_observations(deployment_id);
CREATE TABLE service_probe_runs (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 completed_at timestamptz NOT NULL
);
