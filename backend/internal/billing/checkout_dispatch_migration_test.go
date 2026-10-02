package billing

import (
	"context"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
)

func TestCheckoutDispatchUpgradePreservesPreparedOriginalReceipt(t *testing.T) {
	s, p, owner, _ := preparedCheckout(t)
	ctx := context.Background()
	query := `SELECT json_build_object('requests',(SELECT jsonb_agg(to_jsonb(r)-'dispatch_started_at'-'customer_id'-'checkout_id'-'session_expires_at'-'authority_origin'-'authority_key_sha256' ORDER BY id) FROM billing_checkout_requests r),'events',(SELECT jsonb_agg(to_jsonb(e)-'executor_kind' ORDER BY id) FROM billing_checkout_events e))::text`
	var before, after string
	if err := s.pool.QueryRow(ctx, query).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `DROP TABLE billing_checkout_notifications; ALTER TABLE billing_checkout_requests DROP CONSTRAINT checkout_dispatch_checkpoint; ALTER TABLE billing_checkout_requests DROP CONSTRAINT checkout_authority_identity;
 ALTER TABLE billing_checkout_requests DROP COLUMN dispatch_started_at, DROP COLUMN customer_id, DROP COLUMN checkout_id, DROP COLUMN session_expires_at, DROP COLUMN authority_origin, DROP COLUMN authority_key_sha256;
 ALTER TABLE billing_checkout_requests DROP CONSTRAINT billing_checkout_requests_state_check;
 ALTER TABLE billing_checkout_requests ADD CONSTRAINT billing_checkout_requests_state_check CHECK(state='prepared');
 ALTER TABLE billing_checkout_events DROP CONSTRAINT checkout_executor_kind;
 ALTER TABLE billing_checkout_events DROP COLUMN executor_kind;
 ALTER TABLE billing_checkout_events DROP CONSTRAINT billing_checkout_events_action_check;
 ALTER TABLE billing_checkout_events ADD CONSTRAINT billing_checkout_events_action_check CHECK(action='prepare');
 UPDATE schema_version SET version=31`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrate.Apply(ctx, s.pool); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.pool.QueryRow(ctx, query).Scan(&after); err != nil || after != before {
		t.Fatal("dispatch upgrade changed original preparation or audit")
	}
	receipt, err := s.Get(ctx, owner, p.OrganizationID, p.ID)
	if err != nil || receipt != p {
		t.Fatal("dispatch upgrade changed original safe receipt")
	}
	var untouched bool
	if err = s.pool.QueryRow(ctx, `SELECT dispatch_started_at IS NULL AND customer_id IS NULL AND checkout_id IS NULL AND session_expires_at IS NULL FROM billing_checkout_requests WHERE id=$1`, p.ID).Scan(&untouched); err != nil || !untouched {
		t.Fatal("dispatch migration invented remote work")
	}
	var executor string
	if err = s.pool.QueryRow(ctx, `SELECT executor_kind FROM billing_checkout_events WHERE request_id=$1`, p.ID).Scan(&executor); err != nil || executor != "owner" {
		t.Fatal("dispatch migration changed review attribution")
	}
}
