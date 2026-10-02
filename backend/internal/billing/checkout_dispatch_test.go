package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

type checkoutMock struct {
	mu                              sync.Mutex
	customers                       []string
	sessions                        []SessionRequest
	customerFailure, sessionFailure bool
	onCustomer, onSession           func()
}

func (m *checkoutMock) CreateCustomer(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	m.customers = append(m.customers, key)
	fail, hook := m.customerFailure, m.onCustomer
	m.mu.Unlock()
	if hook != nil {
		hook()
	}
	if fail {
		return "", ErrUnavailable
	}
	return "cus_dispatchFixture", nil
}
func (m *checkoutMock) Checkout(ctx context.Context, r SessionRequest) (HostedSession, error) {
	m.mu.Lock()
	m.sessions = append(m.sessions, r)
	fail, hook := m.sessionFailure, m.onSession
	m.mu.Unlock()
	if hook != nil {
		hook()
	}
	if fail {
		return HostedSession{}, ErrUnavailable
	}
	return HostedSession{ID: "cs_test_dispatchFixture", URL: "http://127.0.0.1/private_navigation_marker", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (m *checkoutMock) counts() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.customers), len(m.sessions)
}

func preparedCheckout(t *testing.T) (*CheckoutIntents, CheckoutIntent, string, *checkoutMock) {
	t.Helper()
	s, _, owner, org, _ := fixtureCheckoutIntents(t)
	p, err := s.Prepare(context.Background(), owner, org, sessionFixtureRequest.Key, true)
	if err != nil {
		t.Fatal(err)
	}
	return s, p, owner, &checkoutMock{}
}

func TestCheckoutStartConcurrentReplayQueuesOneOriginalJob(t *testing.T) {
	s, p, owner, m := preparedCheckout(t)
	ctx := context.Background()
	answers := make(chan error, 8)
	for range 8 {
		go func() {
			r, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true)
			if err == nil && (r.ID != p.ID || r.Key != p.Key || r.State != "queued" || r.CanPay) {
				err = errors.New("changed original dispatch receipt")
			}
			answers <- err
		}()
	}
	for range 8 {
		if err := <-answers; err != nil {
			t.Fatal(err)
		}
	}
	if requests, events, jobs := checkoutCounts(t, s.pool); requests != 1 || events != 2 || jobs != 1 {
		t.Fatal("concurrent start duplicated work")
	}
	if c, k := m.counts(); c != 0 || k != 0 {
		t.Fatal("submission dispatched remote work")
	}
	if err := s.Dispatch(ctx, p.ID, m); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, owner, p.OrganizationID, p.ID)
	if err != nil || got.State != "session_ready" || got.CanPay {
		t.Fatal("session confirmation changed payment authority")
	}
	if err = s.Dispatch(ctx, p.ID, m); err != nil {
		t.Fatal("confirmed dispatch retry failed")
	}
	if _, err = s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
		t.Fatal("confirmed original start retry failed")
	}
	if c, k := m.counts(); c != 1 || k != 1 {
		t.Fatal("confirmed receipt repeated remote calls")
	}
	if _, events, jobs := checkoutCounts(t, s.pool); events != 5 || jobs != 1 {
		t.Fatal("confirmed retry changed audit or queue")
	}
	var private bool
	if err = s.pool.QueryRow(ctx, `SELECT row_to_json(r)::text LIKE '%private_navigation_marker%' FROM billing_checkout_requests r WHERE id=$1`, p.ID).Scan(&private); err != nil || private {
		t.Fatal("navigation capability persisted")
	}
}

func TestCheckoutStartRequiresReviewAndRollsBackQueueOrAuditFailure(t *testing.T) {
	for _, mode := range []string{"unconfirmed", "queue", "audit", "binding"} {
		t.Run(mode, func(t *testing.T) {
			s, p, owner, _ := preparedCheckout(t)
			ctx := context.Background()
			var err error
			switch mode {
			case "queue":
				_, err = s.pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT reject_checkout_fixture CHECK(kind<>'billing_checkout') NOT VALID`)
			case "audit":
				_, err = s.pool.Exec(ctx, `ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_checkout_queue CHECK(action<>'queue') NOT VALID`)
			case "binding":
				_, err = s.pool.Exec(ctx, `INSERT INTO billing_customer_bindings(organization_id,customer_id,subscription_id,idempotency_key,actor,reason) VALUES($1,'cus_existing','sub_existing',$2,'fixture','qualification')`, p.OrganizationID, p.Key)
			}
			if err != nil {
				t.Fatal(err)
			}
			if r, err := s.Start(ctx, owner, p.OrganizationID, p.ID, mode != "unconfirmed"); err == nil || r.ID != "" {
				t.Fatal("unconfirmed queue intent escaped")
			}
			got, err := s.Get(ctx, owner, p.OrganizationID, p.ID)
			if err != nil || got.State != "prepared" {
				t.Fatal("queue failure changed preparation")
			}
			if _, events, jobs := checkoutCounts(t, s.pool); events != 1 || jobs != 0 {
				t.Fatal("queue failure left work or audit")
			}
		})
	}
}

func TestCheckoutLostRemoteOutcomesRetainBoundedOriginalIdentity(t *testing.T) {
	for _, stage := range []string{"customer", "session"} {
		t.Run(stage, func(t *testing.T) {
			s, p, owner, m := preparedCheckout(t)
			ctx := context.Background()
			if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
				t.Fatal(err)
			}
			if stage == "customer" {
				m.customerFailure = true
			} else {
				m.sessionFailure = true
			}
			if err := s.Dispatch(ctx, p.ID, m); err != ErrUnavailable {
				t.Fatal("lost remote result not retained")
			}
			var started time.Time
			var customer *string
			if err := s.pool.QueryRow(ctx, `SELECT dispatch_started_at,customer_id FROM billing_checkout_requests WHERE id=$1`, p.ID).Scan(&started, &customer); err != nil || started.IsZero() {
				t.Fatal("dispatch identity not durable")
			}
			if stage == "customer" && customer != nil || stage == "session" && (customer == nil || *customer != "cus_dispatchFixture") {
				t.Fatal("lost result corrupted customer checkpoint")
			}
			m.mu.Lock()
			m.customerFailure = false
			m.sessionFailure = false
			m.mu.Unlock()
			if err := s.Dispatch(ctx, p.ID, m); err != nil {
				t.Fatal("original bounded retry failed")
			}
			m.mu.Lock()
			for _, key := range m.customers {
				if key != p.ID {
					t.Error("customer retry changed dispatch identity")
				}
			}
			for _, r := range m.sessions {
				if r.Key != p.ID || r.CustomerID != "cus_dispatchFixture" {
					t.Error("session retry changed customer or identity")
				}
			}
			m.mu.Unlock()
			c, k := m.counts()
			if stage == "customer" && (c != 2 || k != 1) || stage == "session" && (c != 1 || k != 2) {
				t.Fatal("retry repeated a confirmed step")
			}
		})
	}
}

func TestCheckoutExpiredUnknownDispatchRequiresManualReview(t *testing.T) {
	s, p, owner, m := preparedCheckout(t)
	ctx := context.Background()
	if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.reserveDispatch(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE billing_checkout_requests SET dispatch_started_at=clock_timestamp()-interval '24 hours' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(ctx, p.ID, m); err != nil {
		t.Fatal(err)
	}
	r, err := s.Get(ctx, owner, p.OrganizationID, p.ID)
	if err != nil || r.State != "manual_review" || r.CanPay {
		t.Fatal("expired request regained payment authority")
	}
	if err = s.Dispatch(ctx, p.ID, m); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if c, k := m.counts(); c != 0 || k != 0 {
		t.Fatal("expired original key created another remote resource")
	}
	if _, events, jobs := checkoutCounts(t, s.pool); events != 4 || jobs != 1 {
		t.Fatal("manual review retry changed work")
	}
}

func TestCheckoutCheckpointFailureRetainsOriginalReservation(t *testing.T) {
	for _, stage := range []string{"customer_confirmed", "session_confirmed"} {
		t.Run(stage, func(t *testing.T) {
			s, p, owner, m := preparedCheckout(t)
			ctx := context.Background()
			if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
				t.Fatal(err)
			}

			statement := "ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_checkpoint_fixture CHECK(action<>'customer_confirmed') NOT VALID"
			if stage == "session_confirmed" {
				statement = "ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_checkpoint_fixture CHECK(action<>'session_confirmed') NOT VALID"
			}
			if _, err := s.pool.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			if err := s.Dispatch(ctx, p.ID, m); err == nil {
				t.Fatal("failed checkpoint escaped")
			}
			r, err := s.Get(ctx, owner, p.OrganizationID, p.ID)
			want := "queued"
			if stage == "session_confirmed" {
				want = "customer_ready"
			}
			if err != nil || r.State != want {
				t.Fatal("failed checkpoint changed confirmed state")
			}
			if _, err = s.pool.Exec(ctx, `ALTER TABLE billing_checkout_events DROP CONSTRAINT reject_checkpoint_fixture`); err != nil {
				t.Fatal(err)
			}
			if err = s.Dispatch(ctx, p.ID, m); err != nil {
				t.Fatal("checkpoint retry failed")
			}
			c, k := m.counts()
			if stage == "customer_confirmed" && (c != 2 || k != 1) || stage == "session_confirmed" && (c != 1 || k != 2) {
				t.Fatal("checkpoint retry replaced original work")
			}
		})
	}
}

func TestCheckoutDisabledWorkerSnoozesWithoutRemoteAuthority(t *testing.T) {
	w := &CheckoutWorker{}
	err := w.Work(context.Background(), &river.Job[CheckoutArgs]{Args: CheckoutArgs{RequestID: sessionFixtureRequest.Key}})
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatal("disabled checkout work did not wait")
	}
}

func TestCheckoutWorkerMissingRecoveryWaitsWithoutDispatch(t *testing.T) {
	s, p, owner, m := preparedCheckout(t)
	ctx := context.Background()
	if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	before := checkoutLedger(t, s)
	if _, err := s.pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
		t.Fatal(err)
	}
	w := &CheckoutWorker{Store: s, Authority: m}
	err := w.Work(ctx, &river.Job[CheckoutArgs]{Args: CheckoutArgs{RequestID: p.ID}})
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatal("missing recovery consumed checkout failure retries")
	}
	if c, k := m.counts(); c != 0 || k != 0 || checkoutLedger(t, s) != before {
		t.Fatal("missing recovery changed checkout work")
	}
}

func TestCheckoutReservationFailureNeverStartsRemoteWork(t *testing.T) {
	for _, mode := range []string{"prepared", "checkpoint", "audit"} {
		t.Run(mode, func(t *testing.T) {
			s, p, owner, m := preparedCheckout(t)
			ctx := context.Background()
			if mode != "prepared" {
				if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			if mode == "checkpoint" {
				_, err = s.pool.Exec(ctx, `ALTER TABLE billing_checkout_requests ADD CONSTRAINT reject_reservation CHECK(dispatch_started_at IS NULL) NOT VALID`)
			}
			if mode == "audit" {
				_, err = s.pool.Exec(ctx, `ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_reservation CHECK(action<>'dispatch_reserve') NOT VALID`)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := checkoutLedger(t, s)
			w := &CheckoutWorker{Store: s, Authority: m}
			if err = w.Work(ctx, &river.Job[CheckoutArgs]{Args: CheckoutArgs{RequestID: p.ID}}); err == nil {
				t.Fatal("failed dispatch reservation completed")
			}
			if c, k := m.counts(); c != 0 || k != 0 || checkoutLedger(t, s) != before {
				t.Fatal("failed reservation dispatched or changed ledger")
			}
		})
	}
}

func TestCheckoutWorkerConfirmsRetainedSessionOnce(t *testing.T) {
	s, p, owner, m := preparedCheckout(t)
	ctx := context.Background()
	if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	w := &CheckoutWorker{Store: s, Authority: m}
	for range 2 {
		if err := w.Work(ctx, &river.Job[CheckoutArgs]{Args: CheckoutArgs{RequestID: p.ID}}); err != nil {
			t.Fatal("confirmed worker result failed")
		}
	}
	if c, k := m.counts(); c != 1 || k != 1 {
		t.Fatal("worker replay repeated a confirmed remote step")
	}
}
