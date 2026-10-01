ALTER TABLE service_operations DROP CONSTRAINT service_operations_reason_check;
ALTER TABLE service_operations ADD CONSTRAINT service_operations_reason_check CHECK(reason IN ('maintenance','safety_review','resource_pressure','customer_exit'));
CREATE TABLE hosting_exits (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 export_id uuid NOT NULL REFERENCES export_operations(id),
 state text NOT NULL DEFAULT 'scheduled' CHECK(state IN ('scheduled','stopping','ended','resuming','reversed','review')),
 service_end_at timestamptz NOT NULL,
 retention_review_at timestamptz NOT NULL,
 delay_seconds integer NOT NULL CHECK(delay_seconds BETWEEN 30 AND 604800),
 retention_days integer NOT NULL CHECK(retention_days BETWEEN 1 AND 30),
 stop_operation_id uuid REFERENCES service_operations(id),
 resume_operation_id uuid REFERENCES service_operations(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(deployment_id,idempotency_key),
 CHECK(service_end_at>=created_at AND retention_review_at=service_end_at+make_interval(hours=>retention_days*24)),
 CHECK(state NOT IN ('stopping','ended','resuming') OR stop_operation_id IS NOT NULL),
 CHECK(state<>'resuming' OR resume_operation_id IS NOT NULL)
);
CREATE UNIQUE INDEX hosting_exits_active ON hosting_exits(deployment_id) WHERE state<>'reversed';
CREATE TABLE hosting_exit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 exit_id uuid NOT NULL REFERENCES hosting_exits(id),
 state text NOT NULL CHECK(state IN ('scheduled','stopping','ended','resuming','reversed','review')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE hosting_exit_reversals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 exit_id uuid NOT NULL REFERENCES hosting_exits(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(exit_id,idempotency_key)
);
