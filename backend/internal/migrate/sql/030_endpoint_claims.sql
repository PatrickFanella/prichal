ALTER TABLE endpoint_choices ADD CONSTRAINT endpoint_choice_claim_identity UNIQUE(id,organization_id,actor_id,hostname);
CREATE TABLE endpoint_claims (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 choice_id uuid NOT NULL UNIQUE,
 organization_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 hostname text NOT NULL,
 proof_sha256 bytea NOT NULL CHECK(octet_length(proof_sha256)=32),
 state text NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','released','expired')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 verified_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 CHECK(verified_at<=created_at AND created_at<=verified_at+interval '8 seconds'),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '7 days'),
 FOREIGN KEY(choice_id,organization_id,actor_id,hostname) REFERENCES endpoint_choices(id,organization_id,actor_id,hostname)
);
CREATE UNIQUE INDEX endpoint_claims_reserved_hostname ON endpoint_claims(hostname) WHERE state='reserved';
CREATE TABLE endpoint_claim_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 claim_id uuid NOT NULL REFERENCES endpoint_claims(id),
 actor_id uuid REFERENCES owners(id),
 action text NOT NULL CHECK(action IN ('reserve','release','expire')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((action='expire' AND actor_id IS NULL) OR (action<>'expire' AND actor_id IS NOT NULL))
);
