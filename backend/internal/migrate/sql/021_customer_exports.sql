CREATE TABLE export_operations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deployment_id uuid NOT NULL REFERENCES deployments(id),
 owner_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 account_did text NOT NULL CHECK(account_did ~ '^did:plc:[a-z2-7]{24}$'),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','ready','failed')),
 archive_bytes bigint CHECK(archive_bytes>0 AND archive_bytes<=20971520),
 archive_sha256 text CHECK(archive_sha256 ~ '^[a-f0-9]{64}$'),
 captured_at timestamptz,
 expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(deployment_id,idempotency_key),
 CHECK((state='ready' AND archive_bytes IS NOT NULL AND archive_sha256 IS NOT NULL AND captured_at IS NOT NULL AND expires_at IS NOT NULL AND expires_at=captured_at+interval '24 hours') OR (state<>'ready' AND archive_bytes IS NULL AND archive_sha256 IS NULL AND captured_at IS NULL AND expires_at IS NULL))
);
CREATE TABLE export_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 export_id uuid NOT NULL REFERENCES export_operations(id),
 state text NOT NULL CHECK(state IN ('queued','ready','failed')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX export_operations_owner ON export_operations(owner_id,created_at DESC);
