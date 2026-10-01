package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixtureStore(t *testing.T) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','billing','Billing') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Billing fixture',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	return store, pool, org
}
func fixtureEvent(t *testing.T, id string, created int64) Event {
	t.Helper()
	raw := fixturePayload(t, id, "customer.subscription.updated", created, fixtureSubscription())
	secret := fixtureSecret(t, "whsec_")
	event, supported, err := Verify(raw, signature(raw, secret, time.Now()), secret)
	if err != nil || !supported {
		t.Fatal("invalid test event")
	}
	return event
}
func TestDurableWebhookDeduplicationAndRollback(t *testing.T) {
	store, pool, _ := fixtureStore(t)
	ctx := context.Background()
	event := fixtureEvent(t, "evt_fixtureConcurrent", time.Now().Unix())
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { _, err := store.Receive(ctx, event); results <- err }()
	}
	for i := 0; i < 8; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var events, jobs int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM billing_events),(SELECT count(*) FROM river_job WHERE kind='billing_reconcile')`).Scan(&events, &jobs); err != nil || events != 1 || jobs != 1 {
		t.Fatal("duplicate notification created duplicate work")
	}
	altered := event
	altered.Customer = "cus_fixtureBob"
	if _, err := store.Receive(ctx, altered); !errors.Is(err, ErrConflict) {
		t.Fatal("event ID accepted changed reconciliation identity")
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT reject_billing CHECK(kind<>'billing_reconcile') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	event = fixtureEvent(t, "evt_fixtureRollback", time.Now().Unix())
	if _, err := store.Receive(ctx, event); err == nil {
		t.Fatal("job failure accepted inbox event")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_events`).Scan(&events); err != nil || events != 1 {
		t.Fatal("inbox event survived failed job transaction")
	}
	var leak bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM billing_events WHERE row_to_json(billing_events)::text LIKE '%private_marker%')`).Scan(&leak); err != nil || leak {
		t.Fatal("raw private payment data retained")
	}
}

type currentAuthority struct {
	snapshot Snapshot
	failure  bool
	calls    int
}

func (a *currentAuthority) Subscription(context.Context, string) (Snapshot, error) {
	a.calls++
	if a.failure {
		return Snapshot{}, ErrUnavailable
	}
	return a.snapshot, nil
}
func bindFixture(t *testing.T, store *Store, org string) {
	t.Helper()
	if err := store.Bind(context.Background(), Binding{OrganizationID: org, CustomerID: "cus_fixtureAlice", SubscriptionID: "sub_fixtureAlice", Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", Actor: "test-operator"}); err != nil {
		t.Fatal(err)
	}
}
func TestReconcileUsesCurrentAuthorityAndDoesNotGrantHosting(t *testing.T) {
	store, pool, org := fixtureStore(t)
	ctx := context.Background()
	bindFixture(t, store, org)
	authority := &currentAuthority{snapshot: Snapshot{Customer: "cus_fixtureAlice", Subscription: "sub_fixtureAlice", State: "active", Price: "price_fixtureSingle", Quantity: 1, PeriodEnd: time.Now().Add(24 * time.Hour)}}
	newer := fixtureEvent(t, "evt_fixtureNewer", time.Now().Unix())
	older := fixtureEvent(t, "evt_fixtureOlder", time.Now().Add(-24*time.Hour).Unix())
	for _, event := range []Event{newer, older} {
		if _, err := store.Receive(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Reconcile(ctx, newer.ID, authority); err != nil {
		t.Fatal(err)
	}
	authority.snapshot.State = "canceled"
	if err := store.Reconcile(ctx, older.ID, authority); err != nil {
		t.Fatal(err)
	}
	var state, eventID string
	if err := pool.QueryRow(ctx, `SELECT subscription_state,event_id FROM billing_subscription_observations WHERE organization_id=$1`, org).Scan(&state, &eventID); err != nil || state != "canceled" || eventID != older.ID {
		t.Fatal("old notification overwrote current authoritative state")
	}
	if err := store.Reconcile(ctx, older.ID, authority); err != nil || authority.calls != 2 {
		t.Fatal("applied event replay repeated authority side effects")
	}
	var grants, deployments int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM pilot_entitlement_operations),(SELECT count(*) FROM deployments)`).Scan(&grants, &deployments); err != nil || grants != 0 || deployments != 0 {
		t.Fatal("fixture observation granted paid hosting or native work")
	}
}
func TestReconcileFailureAndBindingMismatchRemainPending(t *testing.T) {
	store, pool, org := fixtureStore(t)
	ctx := context.Background()
	event := fixtureEvent(t, "evt_fixtureFailure", time.Now().Unix())
	if _, err := store.Receive(ctx, event); err != nil {
		t.Fatal(err)
	}
	authority := &currentAuthority{snapshot: Snapshot{Customer: "cus_fixtureAlice", Subscription: "sub_fixtureAlice", State: "active", Price: "price_fixtureSingle", Quantity: 1, PeriodEnd: time.Now().Add(time.Hour)}}
	if err := store.Reconcile(ctx, event.ID, authority); !errors.Is(err, ErrUnmapped) || authority.calls != 0 {
		t.Fatal("unmapped event reached authority")
	}
	bindFixture(t, store, org)
	authority.failure = true
	if err := store.Reconcile(ctx, event.ID, authority); !errors.Is(err, ErrUnavailable) {
		t.Fatal("authority failure applied event")
	}
	authority.failure = false
	authority.snapshot.Customer = "cus_fixtureBob"
	if err := store.Reconcile(ctx, event.ID, authority); !errors.Is(err, ErrInvalidEvent) {
		t.Fatal("foreign customer authority applied event")
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM billing_events WHERE event_id=$1`, event.ID).Scan(&state); err != nil || state != "pending" {
		t.Fatal("failed reconciliation consumed event")
	}
	authority.snapshot.Customer = "cus_fixtureAlice"
	if err := store.Reconcile(ctx, event.ID, authority); err != nil {
		t.Fatal(err)
	}
	if err := store.Bind(ctx, Binding{OrganizationID: org, CustomerID: "cus_fixtureBob", SubscriptionID: "sub_fixtureBob", Key: "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb", Actor: "operator"}); !errors.Is(err, ErrConflict) {
		t.Fatal("immutable ownership binding changed")
	}
}

type serialAuthority struct {
	mu                sync.Mutex
	active, maxActive int
	snapshot          Snapshot
}

func (a *serialAuthority) Subscription(ctx context.Context, _ string) (Snapshot, error) {
	a.mu.Lock()
	a.active++
	a.maxActive = max(a.maxActive, a.active)
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.active--; a.mu.Unlock() }()
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-time.After(30 * time.Millisecond):
		return a.snapshot, nil
	}
}
func TestConcurrentReconciliationSerializesAuthorityReads(t *testing.T) {
	store, _, org := fixtureStore(t)
	bindFixture(t, store, org)
	ctx := context.Background()
	authority := &serialAuthority{snapshot: Snapshot{Customer: "cus_fixtureAlice", Subscription: "sub_fixtureAlice", State: "past_due", Price: "price_fixtureSingle", Quantity: 1, PeriodEnd: time.Now().Add(time.Hour)}}
	results := make(chan error, 2)
	for _, id := range []string{"evt_fixtureFirst", "evt_fixtureSecond"} {
		event := fixtureEvent(t, id, time.Now().Unix())
		if _, err := store.Receive(ctx, event); err != nil {
			t.Fatal(err)
		}
		go func(id string) { results <- store.Reconcile(ctx, id, authority) }(id)
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if authority.maxActive != 1 {
		t.Fatal("authority reads overlapped for one organization")
	}
}
