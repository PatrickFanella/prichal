CREATE TABLE support_inspections (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_owner_id uuid NOT NULL REFERENCES owners(id),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 deployment_id uuid REFERENCES deployments(id),
 role text NOT NULL CHECK(role IN ('inspect','operate')),
 purpose text NOT NULL DEFAULT 'support' CHECK(purpose='support'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX support_inspections_deployment ON support_inspections(deployment_id,id DESC);
CREATE INDEX support_inspections_organization ON support_inspections(organization_id,id DESC);
