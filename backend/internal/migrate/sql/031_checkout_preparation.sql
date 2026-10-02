-- Fixture-only owner review. Preparation grants no entitlement and dispatches no payment.
CREATE TABLE billing_checkout_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 actor_id uuid NOT NULL REFERENCES owners(id),
 idempotency_key uuid NOT NULL,
 price_id text NOT NULL CHECK(price_id ~ '^price_[A-Za-z0-9]{1,100}$'),
 return_origin text NOT NULL CHECK(length(return_origin) BETWEEN 1 AND 256),
 state text NOT NULL DEFAULT 'prepared' CHECK(state='prepared'),
 source text NOT NULL DEFAULT 'local_fixture' CHECK(source='local_fixture'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(organization_id,idempotency_key),
 UNIQUE(id,organization_id,actor_id),
 UNIQUE(organization_id)
);
CREATE TABLE billing_checkout_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 request_id uuid NOT NULL,
 organization_id uuid NOT NULL,
 actor_id uuid NOT NULL,
 action text NOT NULL CHECK(action='prepare'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(request_id,organization_id,actor_id) REFERENCES billing_checkout_requests(id,organization_id,actor_id),
 UNIQUE(request_id,action)
);
