package testdb

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
	"testing"
	"time"
)

// ReconstructCheckout33 removes only empty verification columns in an allocator-owned
// fixture before historical migration tests reconstruct an earlier schema.
func ReconstructCheckout33(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if !regexp.MustCompile(`^pds_smoke_[0-9a-f]{16}$`).MatchString(pool.Config().ConnConfig.Database) {
		t.Fatal("allocator-owned migration fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var empty bool
	if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM billing_customer_bindings WHERE reason='checkout_verified') AND NOT EXISTS(SELECT 1 FROM billing_checkout_events WHERE completion_event_id IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM billing_subscription_observations WHERE completion_event_id IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM billing_checkout_notifications WHERE state='applied')`).Scan(&empty); err != nil || !empty {
		t.Fatal("populated verification cannot be reconstructed")
	}
	ReconstructMembers34(t, pool)
	if _, err := pool.Exec(ctx, `
 ALTER TABLE billing_subscription_observations DROP CONSTRAINT subscription_observation_delivery;
 ALTER TABLE billing_subscription_observations DROP COLUMN completion_event_id, DROP COLUMN checkout_payment_state;
 ALTER TABLE billing_subscription_observations ALTER COLUMN event_id SET NOT NULL;
 ALTER TABLE billing_checkout_events DROP CONSTRAINT checkout_executor_kind;
 ALTER TABLE billing_checkout_events ADD CONSTRAINT checkout_executor_kind CHECK((action IN ('prepare','queue') AND executor_kind='owner') OR (action IN ('dispatch_reserve','customer_confirmed','session_confirmed','manual_review') AND executor_kind='worker'));
 ALTER TABLE billing_checkout_events DROP CONSTRAINT checkout_completion_audit;
 ALTER TABLE billing_checkout_events DROP COLUMN completion_event_id;
 ALTER TABLE billing_checkout_events DROP CONSTRAINT billing_checkout_events_action_check;
 ALTER TABLE billing_checkout_events ADD CONSTRAINT billing_checkout_events_action_check CHECK(action IN ('prepare','queue','dispatch_reserve','customer_confirmed','session_confirmed','manual_review'));
 ALTER TABLE billing_customer_bindings DROP CONSTRAINT checkout_binding_evidence;
 ALTER TABLE billing_customer_bindings DROP CONSTRAINT checkout_binding_custody;
 ALTER TABLE billing_customer_bindings DROP COLUMN checkout_request_id, DROP COLUMN checkout_actor_id, DROP COLUMN completion_event_id;
 ALTER TABLE billing_customer_bindings DROP CONSTRAINT billing_customer_bindings_reason_check;
 ALTER TABLE billing_customer_bindings ADD CONSTRAINT billing_customer_bindings_reason_check CHECK(reason='qualification');
 ALTER TABLE billing_checkout_notifications DROP CONSTRAINT checkout_notification_receipt;
 ALTER TABLE billing_checkout_notifications DROP COLUMN applied_at;
 ALTER TABLE billing_checkout_notifications DROP CONSTRAINT billing_checkout_notifications_state_check;
 ALTER TABLE billing_checkout_notifications ADD CONSTRAINT billing_checkout_notifications_state_check CHECK(state='pending');
 UPDATE schema_version SET version=33`); err != nil {
		t.Fatal(err)
	}
}

// ReconstructMembers34 removes empty member invitation tables so historical
// migration tests can rebuild version 34 and earlier schemas.
func ReconstructMembers34(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ReconstructManaged38(t, pool)
	if !regexp.MustCompile(`^pds_smoke_[0-9a-f]{16}$`).MatchString(pool.Config().ConnConfig.Database) {
		t.Fatal("allocator-owned migration fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var empty bool
	if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM member_invitations) AND NOT EXISTS(SELECT 1 FROM instance_runtime_releases)`).Scan(&empty); err != nil || !empty {
		t.Fatal("populated member invitations or runtime observations cannot be reconstructed")
	}
	if _, err := pool.Exec(ctx, `DROP TABLE member_removal_events,member_removals; DROP TABLE instance_runtime_releases; DROP TABLE member_invitation_events,member_invitations; UPDATE schema_version SET version=34`); err != nil {
		t.Fatal(err)
	}
}

// ReconstructManaged38 removes only empty public-routing metadata in allocator-owned fixtures.
func ReconstructManaged38(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ReconstructManaged40(t, pool)
	if !regexp.MustCompile(`^pds_smoke_[a-f0-9]{16}$`).MatchString(pool.Config().ConnConfig.Database) {
		t.Fatal("allocator-owned migration fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DO $$ BEGIN IF EXISTS(SELECT 1 FROM managed_native_conversions) THEN RAISE EXCEPTION 'populated managed conversions cannot be reconstructed'; END IF; END $$; DROP TABLE managed_native_conversions`); err != nil {
		t.Fatal(err)
	}
	var empty bool
	if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM managed_public_endpoints) AND NOT EXISTS(SELECT 1 FROM managed_edge_observations)`).Scan(&empty); err != nil || !empty {
		t.Fatal("populated public routing cannot be reconstructed")
	}
	if _, err := pool.Exec(ctx, `DROP TABLE managed_edge_observations,managed_public_endpoints; UPDATE schema_version SET version=38`); err != nil {
		t.Fatal(err)
	}
}

// ReconstructManaged40 removes only empty readiness metadata in an allocator-owned database.
func ReconstructManaged40(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ReconstructManaged41(t, pool)
	if !regexp.MustCompile(`^pds_smoke_[a-f0-9]{16}$`).MatchString(pool.Config().ConnConfig.Database) {
		t.Fatal("allocator-owned migration fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DO $$ BEGIN IF EXISTS(SELECT 1 FROM managed_public_readiness) OR EXISTS(SELECT 1 FROM managed_public_checks) OR EXISTS(SELECT 1 FROM managed_public_retries) THEN RAISE EXCEPTION 'populated public readiness cannot be reconstructed'; END IF; END $$; DROP TABLE managed_public_retries,managed_public_checks,managed_public_readiness`); err != nil {
		t.Fatal(err)
	}
}

// ReconstructManaged41 removes only empty retirement metadata from disposable fixtures.
func ReconstructManaged41(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if !regexp.MustCompile(`^pds_smoke_[a-f0-9]{16}$`).MatchString(pool.Config().ConnConfig.Database) {
		t.Fatal("allocator-owned migration fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DO $$ BEGIN IF EXISTS(SELECT 1 FROM managed_endpoint_retirements) THEN RAISE EXCEPTION 'populated retirements cannot be reconstructed'; END IF; END $$; DROP TABLE managed_endpoint_retirements`); err != nil {
		t.Fatal(err)
	}
}
