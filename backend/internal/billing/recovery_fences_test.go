package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/riverqueue/river"
)

func billingLedger(t *testing.T, s *Store) string {
	t.Helper()
	var rows string
	if err := s.Pool.QueryRow(context.Background(), `SELECT json_build_object('events',(SELECT json_agg(row_to_json(x) ORDER BY event_id) FROM billing_events x),'bindings',(SELECT json_agg(row_to_json(x) ORDER BY organization_id) FROM billing_customer_bindings x),'observations',(SELECT json_agg(row_to_json(x) ORDER BY organization_id) FROM billing_subscription_observations x),'jobs',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM river_job x))::text`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}
func holdBillingFixture(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(), `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
}
func billingSnapshotFixture(t *testing.T) (*Store, string, Event, *currentAuthority) {
	t.Helper()
	s, _, org := fixtureStore(t)
	bindFixture(t, s, org)
	event := fixtureEvent(t, "evt_fixtureRecoveryFence", time.Now().Unix())
	if _, err := s.Receive(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	a := &currentAuthority{snapshot: Snapshot{Customer: "cus_fixtureAlice", Subscription: "sub_fixtureAlice", State: "active", Price: "price_fixtureSingle", Quantity: 1, PeriodEnd: time.Now().Add(time.Hour)}}
	return s, org, event, a
}
func TestBillingHeldOrMissingLedgerPreservesInboxBindingsAndWorker(t *testing.T) {
	for _, mode := range []string{"held", "missing"} {
		t.Run(mode, func(t *testing.T) {
			s, org, event, a := billingSnapshotFixture(t)
			if mode == "held" {
				holdBillingFixture(t, s)
			} else {
				if _, err := s.Pool.Exec(context.Background(), `DROP TABLE management_recovery`); err != nil {
					t.Fatal(err)
				}
			}
			before := billingLedger(t, s)
			if duplicate, err := s.Receive(context.Background(), event); err == nil || duplicate {
				t.Fatal("uncertain ledger acknowledged duplicate")
			}
			next := event
			next.ID = "evt_fixtureRecoveryNew"
			if _, err := s.Receive(context.Background(), next); err == nil {
				t.Fatal("uncertain ledger accepted notification")
			}
			if err := s.Bind(context.Background(), Binding{OrganizationID: org, CustomerID: "cus_fixtureAlice", SubscriptionID: "sub_fixtureAlice", Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", Actor: "test-operator"}); err == nil {
				t.Fatal("uncertain ledger accepted binding replay")
			}
			worker := &Worker{Store: s, Authority: a}
			err := worker.Work(context.Background(), &river.Job[Args]{Args: Args{EventID: event.ID}})
			var snooze *river.JobSnoozeError
			if !errors.As(err, &snooze) || a.calls != 0 || billingLedger(t, s) != before {
				t.Fatal("uncertain ledger consumed work or changed retained state")
			}
		})
	}
}

func TestBillingReconcileHoldDuringEventOrOrganizationWait(t *testing.T) {
	for _, mode := range []string{"event", "organization", "applied"} {
		t.Run(mode, func(t *testing.T) {
			s, org, event, a := billingSnapshotFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			if mode == "applied" {
				if err := s.Reconcile(ctx, event.ID, a); err != nil {
					t.Fatal(err)
				}
				a.calls = 0
			}
			before := billingLedger(t, s)
			tx, err := s.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			query := "SELECT customer_id,subscription_id,state"
			if mode == "organization" {
				_, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org)
				query = "SELECT id FROM organizations"
			} else {
				_, err = tx.Exec(ctx, `SELECT event_id FROM billing_events WHERE event_id=$1 FOR UPDATE`, event.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- s.Reconcile(ctx, event.ID, a) }()
			waitCheckoutLock(t, s.Pool, query)
			holdBillingFixture(t, s)
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-done; !errors.Is(err, recovery.ErrHeld) || a.calls != 0 || billingLedger(t, s) != before {
				t.Fatal("lock wait bypassed billing recovery")
			}
		})
	}
}

type heldReadAuthority struct {
	store    *Store
	calls    int
	snapshot Snapshot
	t        *testing.T
}

func (a *heldReadAuthority) Subscription(context.Context, string) (Snapshot, error) {
	a.calls++
	holdBillingFixture(a.t, a.store)
	return a.snapshot, nil
}
func TestBillingHoldDuringAuthorizedReadDiscardsObservation(t *testing.T) {
	s, _, event, a := billingSnapshotFixture(t)
	before := billingLedger(t, s)
	authority := &heldReadAuthority{store: s, snapshot: a.snapshot, t: t}
	if err := s.Reconcile(context.Background(), event.ID, authority); !errors.Is(err, recovery.ErrHeld) || authority.calls != 1 || billingLedger(t, s) != before {
		t.Fatal("held provider response committed subscription observation")
	}
}

func TestBillingHoldDuringObservationWaitRollsBackEventAndObservation(t *testing.T) {
	s, org, first, a := billingSnapshotFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := s.Reconcile(ctx, first.ID, a); err != nil {
		t.Fatal(err)
	}
	next := fixtureEvent(t, "evt_fixtureRecoveryLater", time.Now().Unix())
	if _, err := s.Receive(ctx, next); err != nil {
		t.Fatal(err)
	}
	before := billingLedger(t, s)
	a.calls = 0
	a.snapshot.State = "canceled"
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT organization_id FROM billing_subscription_observations WHERE organization_id=$1 FOR UPDATE`, org); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Reconcile(ctx, next.ID, a) }()
	waitCheckoutLock(t, s.Pool, "INSERT INTO billing_subscription_observations")
	holdBillingFixture(t, s)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, recovery.ErrHeld) || a.calls != 1 || billingLedger(t, s) != before {
		t.Fatal("late observation wait consumed event or retained read")
	}
}

func TestBillingBindingHoldDuringCustodyWaitPreservesMapping(t *testing.T) {
	s, org, _, _ := billingSnapshotFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	before := billingLedger(t, s)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- s.Bind(ctx, Binding{OrganizationID: org, CustomerID: "cus_fixtureAlice", SubscriptionID: "sub_fixtureAlice", Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", Actor: "test-operator"})
	}()
	waitCheckoutLock(t, s.Pool, "SELECT id FROM organizations")
	holdBillingFixture(t, s)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, recovery.ErrHeld) || billingLedger(t, s) != before {
		t.Fatal("held binding replay acknowledged old scope")
	}
}

func TestBillingNotificationHoldDuringInsertRollsBackQueue(t *testing.T) {
	s, _, event, _ := billingSnapshotFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if _, err := s.Pool.Exec(ctx, `CREATE FUNCTION block_billing_fixture_insert() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(42001); RETURN NEW; END$$; CREATE TRIGGER block_billing_fixture_insert BEFORE INSERT ON billing_events FOR EACH ROW EXECUTE FUNCTION block_billing_fixture_insert()`); err != nil {
		t.Fatal(err)
	}
	before := billingLedger(t, s)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(42001)`); err != nil {
		t.Fatal(err)
	}
	event.ID = "evt_fixtureRecoveryInsert"
	done := make(chan error, 1)
	go func() { _, err := s.Receive(ctx, event); done <- err }()
	waitCheckoutLock(t, s.Pool, "INSERT INTO billing_events")
	holdBillingFixture(t, s)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, recovery.ErrHeld) || billingLedger(t, s) != before {
		t.Fatal("held notification insert retained event/job")
	}
}
