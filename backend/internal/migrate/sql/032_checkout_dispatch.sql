ALTER TABLE billing_checkout_requests DROP CONSTRAINT billing_checkout_requests_state_check;
ALTER TABLE billing_checkout_requests ADD CONSTRAINT billing_checkout_requests_state_check CHECK(state IN ('prepared','queued','customer_ready','session_ready','manual_review'));
ALTER TABLE billing_checkout_requests ADD COLUMN dispatch_started_at timestamptz;
ALTER TABLE billing_checkout_requests ADD COLUMN customer_id text UNIQUE CHECK(customer_id IS NULL OR customer_id ~ '^cus_[A-Za-z0-9]{1,100}$');
ALTER TABLE billing_checkout_requests ADD COLUMN checkout_id text UNIQUE CHECK(checkout_id IS NULL OR checkout_id ~ '^cs_test_[A-Za-z0-9]{1,100}$');
ALTER TABLE billing_checkout_requests ADD COLUMN session_expires_at timestamptz;
ALTER TABLE billing_checkout_requests ADD COLUMN authority_origin text CHECK(authority_origin IS NULL OR length(authority_origin) BETWEEN 1 AND 256);
ALTER TABLE billing_checkout_requests ADD COLUMN authority_key_sha256 bytea CHECK(authority_key_sha256 IS NULL OR octet_length(authority_key_sha256)=32);
ALTER TABLE billing_checkout_requests ADD CONSTRAINT checkout_authority_identity CHECK(
 (state='prepared' AND authority_origin IS NULL AND authority_key_sha256 IS NULL) OR
 (state<>'prepared' AND authority_origin IS NOT NULL AND authority_key_sha256 IS NOT NULL)
);
ALTER TABLE billing_checkout_requests ADD CONSTRAINT checkout_dispatch_checkpoint CHECK(
 (state='prepared' AND dispatch_started_at IS NULL AND customer_id IS NULL AND checkout_id IS NULL AND session_expires_at IS NULL) OR
 (state='queued' AND customer_id IS NULL AND checkout_id IS NULL AND session_expires_at IS NULL) OR
 (state='customer_ready' AND dispatch_started_at IS NOT NULL AND customer_id IS NOT NULL AND checkout_id IS NULL AND session_expires_at IS NULL) OR
 (state='session_ready' AND dispatch_started_at IS NOT NULL AND customer_id IS NOT NULL AND checkout_id IS NOT NULL AND session_expires_at IS NOT NULL) OR
 (state='manual_review' AND dispatch_started_at IS NOT NULL AND checkout_id IS NULL AND session_expires_at IS NULL)
);
ALTER TABLE billing_checkout_events DROP CONSTRAINT billing_checkout_events_action_check;
ALTER TABLE billing_checkout_events ADD CONSTRAINT billing_checkout_events_action_check CHECK(action IN ('prepare','queue','dispatch_reserve','customer_confirmed','session_confirmed','manual_review'));
ALTER TABLE billing_checkout_events ADD COLUMN executor_kind text NOT NULL DEFAULT 'owner';
ALTER TABLE billing_checkout_events ADD CONSTRAINT checkout_executor_kind CHECK(
 (action IN ('prepare','queue') AND executor_kind='owner') OR
 (action IN ('dispatch_reserve','customer_confirmed','session_confirmed','manual_review') AND executor_kind='worker')
);
