CREATE TABLE pilot_entitlement_operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id),
    idempotency_key uuid NOT NULL,
    generation bigint NOT NULL CHECK(generation>0),
    state text NOT NULL CHECK(state IN ('active','revoked')),
    plan text CHECK(plan IN ('local-single','local-small')),
    duration_hours integer NOT NULL,
    reason text NOT NULL CHECK(reason IN ('pilot','qualification')),
    actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    UNIQUE(organization_id,idempotency_key),
    UNIQUE(organization_id,generation),
    CHECK((state='active' AND plan IS NOT NULL AND duration_hours BETWEEN 1 AND 720 AND expires_at IS NOT NULL) OR (state='revoked' AND plan IS NULL AND duration_hours=0 AND expires_at IS NULL))
);
-- No implicit exception is assigned to existing or newly created organizations.
-- Expiry/revocation restrict only new native deployments; no PDS resources are deleted.
