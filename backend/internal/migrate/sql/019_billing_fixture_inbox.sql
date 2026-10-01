-- Fixture-only reconciliation evidence. No row in these tables grants paid entitlement.
CREATE TABLE billing_customer_bindings (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id),
    customer_id text UNIQUE NOT NULL CHECK(customer_id ~ '^cus_[A-Za-z0-9]{1,100}$'),
    subscription_id text UNIQUE NOT NULL CHECK(subscription_id ~ '^sub_[A-Za-z0-9]{1,100}$'),
    idempotency_key uuid UNIQUE NOT NULL,
    actor text NOT NULL CHECK(length(actor) BETWEEN 1 AND 100),
    reason text NOT NULL CHECK(reason='qualification'),
    source text NOT NULL DEFAULT 'local_fixture' CHECK(source='local_fixture'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE billing_events (
    event_id text PRIMARY KEY CHECK(event_id ~ '^evt_[A-Za-z0-9]{1,100}$'),
    payload_sha256 bytea NOT NULL CHECK(octet_length(payload_sha256)=32),
    event_type text NOT NULL,
    event_created bigint NOT NULL CHECK(event_created>0),
    customer_id text NOT NULL CHECK(customer_id ~ '^cus_[A-Za-z0-9]{1,100}$'),
    subscription_id text NOT NULL CHECK(subscription_id ~ '^sub_[A-Za-z0-9]{1,100}$'),
    state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','applied')),
    source text NOT NULL DEFAULT 'local_fixture' CHECK(source='local_fixture'),
    received_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    CHECK((state='pending' AND applied_at IS NULL) OR (state='applied' AND applied_at IS NOT NULL))
);
CREATE TABLE billing_subscription_observations (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id),
    customer_id text NOT NULL,
    subscription_id text NOT NULL,
    subscription_state text NOT NULL CHECK(subscription_state IN ('incomplete','incomplete_expired','trialing','active','past_due','canceled','unpaid','paused')),
    price_id text NOT NULL CHECK(price_id ~ '^price_[A-Za-z0-9]{1,100}$'),
    quantity integer NOT NULL CHECK(quantity=1),
    period_end timestamptz NOT NULL,
    event_id text NOT NULL REFERENCES billing_events(event_id),
    source text NOT NULL DEFAULT 'local_fixture' CHECK(source='local_fixture'),
    checked_at timestamptz NOT NULL DEFAULT now()
);
