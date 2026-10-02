-- Signed claims remain pending. They do not select an organization or grant access.
CREATE TABLE billing_checkout_notifications (
 event_id text PRIMARY KEY CHECK(event_id ~ '^evt_[A-Za-z0-9]{1,100}$'),
 event_type text NOT NULL CHECK(event_type IN ('checkout.session.completed','checkout.session.async_payment_succeeded','checkout.session.async_payment_failed')),
 event_created bigint NOT NULL CHECK(event_created>0),
 payload_sha256 bytea NOT NULL CHECK(octet_length(payload_sha256)=32),
 checkout_id text NOT NULL CHECK(checkout_id ~ '^cs_test_[A-Za-z0-9]{1,100}$'),
 customer_id text NOT NULL CHECK(customer_id ~ '^cus_[A-Za-z0-9]{1,100}$'),
 subscription_id text NOT NULL CHECK(subscription_id ~ '^sub_[A-Za-z0-9]{1,100}$'),
 request_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state='pending'),
 source text NOT NULL DEFAULT 'local_fixture' CHECK(source='local_fixture'),
 received_at timestamptz NOT NULL DEFAULT now()
);
