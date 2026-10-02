-- Verified fixture identity/observations do not grant commercial hosting access.
ALTER TABLE billing_checkout_notifications DROP CONSTRAINT billing_checkout_notifications_state_check;
ALTER TABLE billing_checkout_notifications ADD CONSTRAINT billing_checkout_notifications_state_check CHECK(state IN ('pending','applied'));
ALTER TABLE billing_checkout_notifications ADD COLUMN applied_at timestamptz;
ALTER TABLE billing_checkout_notifications ADD CONSTRAINT checkout_notification_receipt CHECK((state='pending' AND applied_at IS NULL) OR (state='applied' AND applied_at IS NOT NULL));

ALTER TABLE billing_customer_bindings DROP CONSTRAINT billing_customer_bindings_reason_check;
ALTER TABLE billing_customer_bindings ADD CONSTRAINT billing_customer_bindings_reason_check CHECK(reason IN ('qualification','checkout_verified'));
ALTER TABLE billing_customer_bindings ADD COLUMN checkout_request_id uuid UNIQUE;
ALTER TABLE billing_customer_bindings ADD COLUMN checkout_actor_id uuid;
ALTER TABLE billing_customer_bindings ADD COLUMN completion_event_id text UNIQUE REFERENCES billing_checkout_notifications(event_id);
ALTER TABLE billing_customer_bindings ADD CONSTRAINT checkout_binding_custody FOREIGN KEY(checkout_request_id,organization_id,checkout_actor_id) REFERENCES billing_checkout_requests(id,organization_id,actor_id);
ALTER TABLE billing_customer_bindings ADD CONSTRAINT checkout_binding_evidence CHECK(
 (reason='qualification' AND checkout_request_id IS NULL AND checkout_actor_id IS NULL AND completion_event_id IS NULL) OR
 (reason='checkout_verified' AND checkout_request_id IS NOT NULL AND checkout_actor_id IS NOT NULL AND completion_event_id IS NOT NULL AND idempotency_key=checkout_request_id AND actor='billing_completion_worker')
);

ALTER TABLE billing_checkout_events DROP CONSTRAINT billing_checkout_events_action_check;
ALTER TABLE billing_checkout_events ADD CONSTRAINT billing_checkout_events_action_check CHECK(action IN ('prepare','queue','dispatch_reserve','customer_confirmed','session_confirmed','manual_review','completion_bound'));
ALTER TABLE billing_checkout_events DROP CONSTRAINT checkout_executor_kind;
ALTER TABLE billing_checkout_events ADD CONSTRAINT checkout_executor_kind CHECK((action IN ('prepare','queue') AND executor_kind='owner') OR (action IN ('dispatch_reserve','customer_confirmed','session_confirmed','manual_review','completion_bound') AND executor_kind='worker'));
ALTER TABLE billing_checkout_events ADD COLUMN completion_event_id text UNIQUE REFERENCES billing_checkout_notifications(event_id);
ALTER TABLE billing_checkout_events ADD CONSTRAINT checkout_completion_audit CHECK((action='completion_bound' AND completion_event_id IS NOT NULL AND executor_kind='worker') OR (action<>'completion_bound' AND completion_event_id IS NULL));

ALTER TABLE billing_subscription_observations ALTER COLUMN event_id DROP NOT NULL;
ALTER TABLE billing_subscription_observations ADD COLUMN completion_event_id text REFERENCES billing_checkout_notifications(event_id);
ALTER TABLE billing_subscription_observations ADD COLUMN checkout_payment_state text CHECK(checkout_payment_state IN ('paid','unpaid','no_payment_required'));
ALTER TABLE billing_subscription_observations ADD CONSTRAINT subscription_observation_delivery CHECK((event_id IS NOT NULL AND completion_event_id IS NULL AND checkout_payment_state IS NULL) OR (event_id IS NULL AND completion_event_id IS NOT NULL AND checkout_payment_state IS NOT NULL));
