package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
)

type completionMappingFixture struct {
	inbox   *CompletionInbox
	intents *CheckoutIntents
	adapter *FixtureSessions
	legacy  *Store
	receipt CheckoutIntent
	owner   string
	expires time.Time
	calls   atomic.Int32
	state   atomic.Value
	mode    atomic.Value
	request atomic.Value
}

func mappingFixture(t *testing.T) *completionMappingFixture {
	t.Helper()
	legacy, pool, org := fixtureStore(t)
	f := &completionMappingFixture{legacy: legacy, expires: time.Now().Add(time.Hour).Truncate(time.Second)}
	f.state.Store("active")
	f.mode.Store("")
	f.request.Store("")
	var origin string
	a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Method == "POST" && r.URL.Path == "/v1/customers" {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": sessionFixtureRequest.CustomerID, "object": "customer", "livemode": false})
			return
		}
		var object map[string]any
		if r.URL.Path == "/v1/subscriptions/sub_completionFixture" {
			object = completionSubscriptionObject(f.expires)
			object["status"] = f.state.Load().(string)
			if f.mode.Load() == "price" {
				object["items"].(map[string]any)["data"].([]any)[0].(map[string]any)["price"].(map[string]any)["id"] = "price_other"
			}
			if f.mode.Load() == "hold_response" {
				holdBillingFixture(t, legacy)
			}
		} else {
			object = recoverySessionResponse(origin, "cs_test_fixtureSessions", f.request.Load().(string), sessionFixtureRequest.CustomerID, f.expires)
			if r.Method == "GET" {
				object["status"], object["subscription"], object["payment_status"] = "complete", "sub_completionFixture", "unpaid"
			}
		}
		_ = json.NewEncoder(w).Encode(object)
	})
	origin = server.URL
	a.settings.Billing.WebhookSecret = fixtureSecret(t, "whsec_")
	f.adapter = a
	var err error
	f.intents, err = NewCheckoutIntents(true, pool, a)
	if err != nil {
		t.Fatal(err)
	}
	f.inbox, err = NewCompletionInbox(true, pool, a)
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&f.owner); err != nil {
		t.Fatal(err)
	}
	f.receipt, err = f.intents.Prepare(context.Background(), f.owner, org, sessionFixtureRequest.Key, true)
	if err != nil {
		t.Fatal(err)
	}
	f.request.Store(f.receipt.ID)
	if _, err = f.intents.Start(context.Background(), f.owner, org, f.receipt.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = f.intents.Dispatch(context.Background(), f.receipt.ID, a); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *completionMappingFixture) event(t *testing.T, id, kind string) CompletionEvent {
	t.Helper()
	object := completionFixtureObject()
	object["id"], object["client_reference_id"] = "cs_test_fixtureSessions", f.receipt.ID
	raw := fixturePayload(t, id, kind, time.Now().Unix(), object)
	e, ok, err := VerifyCompletion(raw, signature(raw, f.inbox.secret, time.Now()), f.inbox.secret)
	if err != nil || !ok {
		t.Fatal("mapping event invalid")
	}
	return e
}
func (f *completionMappingFixture) ledger(t *testing.T) string {
	t.Helper()
	var rows string
	if err := f.inbox.pool.QueryRow(context.Background(), `SELECT json_build_object('bindings',(SELECT jsonb_agg(to_jsonb(b) ORDER BY organization_id) FROM billing_customer_bindings b),'observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY organization_id) FROM billing_subscription_observations o),'notifications',(SELECT jsonb_agg(to_jsonb(n) ORDER BY event_id) FROM billing_checkout_notifications n),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM billing_events e))::text`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows + checkoutLedger(t, f.intents)
}
func TestCompletionMappingAtomicOriginalOwnerCurrentGraphAndDeliveryReplay(t *testing.T) {
	f := mappingFixture(t)
	ctx := context.Background()
	for i, kind := range []string{"checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed"} {
		id := []string{"evt_mappingOne", "evt_mappingTwo", "evt_mappingThree"}[i]
		e := f.event(t, id, kind)
		if _, err := f.inbox.Receive(ctx, e); err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			f.state.Store("past_due")
		}
		if err := f.inbox.Reconcile(ctx, id, f.intents, f.adapter); err != nil {
			t.Fatal(err)
		}
		before := f.ledger(t)
		calls := f.calls.Load()
		if err := f.inbox.Reconcile(ctx, id, f.intents, f.adapter); err != nil || f.ledger(t) != before || f.calls.Load() != calls {
			t.Fatal("applied notification replay changed evidence")
		}
	}
	var valid bool
	err := f.inbox.pool.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_customer_bindings WHERE organization_id=$1 AND reason='checkout_verified' AND checkout_request_id=$2 AND checkout_actor_id=$3 AND actor='billing_completion_worker' AND completion_event_id='evt_mappingOne') AND (SELECT count(*)=1 FROM billing_checkout_events WHERE request_id=$2 AND actor_id=$3 AND executor_kind='worker' AND action='completion_bound' AND completion_event_id='evt_mappingOne') AND (SELECT count(*)=3 FROM billing_checkout_notifications WHERE state='applied') AND (SELECT count(*)=1 FROM billing_subscription_observations WHERE organization_id=$1 AND subscription_state='past_due' AND checkout_payment_state='unpaid' AND event_id IS NULL AND completion_event_id='evt_mappingThree') AND NOT EXISTS(SELECT 1 FROM pilot_entitlement_operations)`, f.receipt.OrganizationID, f.receipt.ID, f.owner).Scan(&valid)
	if err != nil || !valid || f.calls.Load() != 8 {
		t.Fatal("mapping lost original audit/current observation or granted access", err)
	}
	// A legacy subscription delivery refreshes the same observation through original authority scope.
	event := fixtureEvent(t, "evt_mappedLegacy", time.Now().Unix())
	event.Customer = sessionFixtureRequest.CustomerID
	event.Subscription = "sub_completionFixture"
	if _, err = f.legacy.Receive(ctx, event); err != nil {
		t.Fatal(err)
	}
	authority, err := NewAuthority(f.adapter.settings.Billing)
	if err != nil {
		t.Fatal(err)
	}
	f.state.Store("canceled")
	if err = f.legacy.Reconcile(ctx, event.ID, authority); err != nil {
		t.Fatal(err)
	}
	if err = f.inbox.pool.QueryRow(ctx, `SELECT subscription_state='canceled' AND event_id=$1 AND completion_event_id IS NULL AND checkout_payment_state IS NULL FROM billing_subscription_observations WHERE organization_id=$2`, event.ID, f.receipt.OrganizationID).Scan(&valid); err != nil || !valid {
		t.Fatal("legacy delivery did not replace current evidence safely")
	}
}
func TestCompletionMappingRejectsUnmappedCustodyScopeAndGraph(t *testing.T) {
	for _, mode := range []string{"unknown", "customer", "session", "owner", "handover", "configuration", "return", "authority", "key", "binding", "collision", "queued", "customer_ready", "price", "held", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f := mappingFixture(t)
			ctx := context.Background()
			e := f.event(t, "evt_mappingRefused", "checkout.session.completed")
			var err error
			switch mode {
			case "unknown":
				e.Request = sessionFixtureRequest.Key
			case "customer":
				e.Customer = "cus_foreign"
			case "session":
				e.Checkout = "cs_test_foreign"
			case "owner":
				_, err = f.inbox.pool.Exec(ctx, `WITH next AS (INSERT INTO owners(issuer,subject,display_name) VALUES('local','new-mapping-owner','Next') RETURNING id) UPDATE organizations SET owner_id=(SELECT id FROM next) WHERE id=$1`, f.receipt.OrganizationID)
			case "handover":
				_, err = f.inbox.pool.Exec(ctx, `WITH next AS (INSERT INTO owners(issuer,subject,display_name) VALUES('local','handover-mapping','Next') RETURNING id) INSERT INTO organization_transfers(organization_id,previous_owner_id,next_owner_id,idempotency_key) SELECT $1,$2,id,gen_random_uuid() FROM next`, f.receipt.OrganizationID, f.owner)
			case "configuration", "return", "authority", "key":
				settings := f.adapter.settings
				switch mode {
				case "configuration":
					settings.PriceID = "price_changed"
				case "return":
					settings.ReturnOrigin = "http://127.0.0.1:5180"
				case "authority":
					settings.Billing.FixtureURL = "http://127.0.0.1:1"
				case "key":
					err = os.WriteFile(settings.Billing.FixtureKeyFile, []byte(fixtureSecret(t, "rk_test_")), 0600)
				}
				if err == nil {
					f.adapter, err = NewFixtureSessions(settings)
				}
				if err == nil {
					f.intents, err = NewCheckoutIntents(true, f.inbox.pool, f.adapter)
				}
			case "binding":
				err = f.legacy.Bind(ctx, Binding{OrganizationID: f.receipt.OrganizationID, CustomerID: e.Customer, SubscriptionID: e.Subscription, Key: sessionFixtureRequest.Key, Actor: "qualification"})
			case "collision":
				var other string
				if err = f.inbox.pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,'Other binding') RETURNING id`, f.owner).Scan(&other); err == nil {
					err = f.legacy.Bind(ctx, Binding{OrganizationID: other, CustomerID: e.Customer, SubscriptionID: e.Subscription, Key: sessionFixtureRequest.Key, Actor: "qualification"})
				}
			case "queued":
				_, err = f.inbox.pool.Exec(ctx, `UPDATE billing_checkout_requests SET state='queued',customer_id=NULL,checkout_id=NULL,session_expires_at=NULL WHERE id=$1`, f.receipt.ID)
			case "customer_ready":
				_, err = f.inbox.pool.Exec(ctx, `UPDATE billing_checkout_requests SET state='customer_ready',checkout_id=NULL,session_expires_at=NULL WHERE id=$1`, f.receipt.ID)
			case "price":
				f.mode.Store("price")
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.inbox.Receive(ctx, e); err != nil {
				t.Fatal(err)
			}
			if mode == "held" {
				holdBillingFixture(t, f.legacy)
			}
			if mode == "missing" {
				if _, err = f.inbox.pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
					t.Fatal(err)
				}
			}
			before := f.ledger(t)
			calls := f.calls.Load()
			if err = f.inbox.Reconcile(ctx, e.ID, f.intents, f.adapter); err == nil || f.ledger(t) != before {
				t.Fatal("refused mapping changed evidence")
			}
			want := calls
			if mode == "price" {
				want += 2
			}
			if f.calls.Load() != want {
				t.Fatal("uncertain scope called provider")
			}
			worker := &CompletionWorker{Inbox: f.inbox, Intents: f.intents, Authority: f.adapter}
			if err = worker.Work(ctx, &river.Job[CompletionArgs]{Args: CompletionArgs{EventID: e.ID}}); err == nil {
				t.Fatal("refused worker consumed pending work")
			}
		})
	}
}
func TestCompletionMappingWriteFailureAndLateRecoveryRollback(t *testing.T) {
	for _, mode := range []string{"audit", "observation", "receipt", "hold_response", "org_wait", "event_wait", "request_wait", "binding_wait", "observation_wait"} {
		t.Run(mode, func(t *testing.T) {
			f := mappingFixture(t)
			ctx := context.Background()
			e := f.event(t, "evt_mappingRollback", "checkout.session.completed")
			if _, err := f.inbox.Receive(ctx, e); err != nil {
				t.Fatal(err)
			}
			before := f.ledger(t)
			calls := f.calls.Load()
			switch mode {
			case "audit":
				if _, err := f.inbox.pool.Exec(ctx, `ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_mapping CHECK(action<>'completion_bound') NOT VALID`); err != nil {
					t.Fatal(err)
				}
			case "observation":
				if _, err := f.inbox.pool.Exec(ctx, `ALTER TABLE billing_subscription_observations ADD CONSTRAINT reject_mapping CHECK(completion_event_id IS NULL)`); err != nil {
					t.Fatal(err)
				}
			case "receipt":
				if _, err := f.inbox.pool.Exec(ctx, `ALTER TABLE billing_checkout_notifications ADD CONSTRAINT reject_mapping CHECK(state='pending')`); err != nil {
					t.Fatal(err)
				}
			case "hold_response":
				f.mode.Store("hold_response")
			}
			if strings.HasSuffix(mode, "_wait") {
				lock, err := f.inbox.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Rollback(ctx)
				query := `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`
				param := f.receipt.OrganizationID
				match := "SELECT owner_id FROM organizations"
				if mode == "event_wait" {
					query = `SELECT event_id FROM billing_checkout_notifications WHERE event_id=$1 FOR UPDATE`
					param = e.ID
					match = "SELECT request_id::text,checkout_id"
				}
				if mode == "request_wait" {
					query = `SELECT id FROM billing_checkout_requests WHERE id=$1 FOR UPDATE`
					param = f.receipt.ID
					match = "SELECT actor_id,price_id"
				}
				if mode == "binding_wait" || mode == "observation_wait" {
					table := "billing_customer_bindings"
					match = "INSERT INTO billing_customer_bindings"
					if mode == "observation_wait" {
						table = "billing_subscription_observations"
						match = "INSERT INTO billing_subscription_observations"
					}
					if _, err = lock.Exec(ctx, `LOCK TABLE `+table+` IN SHARE MODE`); err != nil {
						t.Fatal(err)
					}
				} else if _, err = lock.Exec(ctx, query, param); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- f.inbox.Reconcile(ctx, e.ID, f.intents, f.adapter) }()
				waitCheckoutLock(t, f.inbox.pool, match)
				holdBillingFixture(t, f.legacy)
				if err = lock.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if err = <-done; err == nil {
					t.Fatal("late custody lock hold accepted mapping")
				}
			} else if err := f.inbox.Reconcile(ctx, e.ID, f.intents, f.adapter); err == nil {
				t.Fatal("uncommitted mapping acknowledged")
			}
			if f.ledger(t) != before {
				t.Fatal("failed mapping retained partial binding/audit/observation/receipt")
			}
			want := calls + 2
			if mode == "org_wait" || mode == "event_wait" || mode == "request_wait" {
				want = calls
			}
			if f.calls.Load() != want {
				t.Fatal("late hold made uncertain request")
			}
		})
	}
}
func TestCompletionMappingUpgradePreservesQualificationAndPendingDelivery(t *testing.T) {
	f := mappingFixture(t)
	ctx := context.Background()
	e := f.event(t, "evt_mappingUpgrade", "checkout.session.completed")
	if _, err := f.inbox.Receive(ctx, e); err != nil {
		t.Fatal(err)
	}
	var other string
	if err := f.inbox.pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,'Upgrade qualification') RETURNING id`, f.owner).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if err := f.legacy.Bind(ctx, Binding{OrganizationID: other, CustomerID: "cus_upgradeLegacy", SubscriptionID: "sub_upgradeLegacy", Key: sessionFixtureRequest.Key, Actor: "qualification"}); err != nil {
		t.Fatal(err)
	}
	legacyEvent := fixtureEvent(t, "evt_upgradeLegacy", time.Now().Unix())
	legacyEvent.Customer = "cus_upgradeLegacy"
	legacyEvent.Subscription = "sub_upgradeLegacy"
	if _, err := f.legacy.Receive(ctx, legacyEvent); err != nil {
		t.Fatal(err)
	}
	legacyAuthority := &currentAuthority{snapshot: Snapshot{Customer: legacyEvent.Customer, Subscription: legacyEvent.Subscription, State: "active", Price: "price_upgradeLegacy", Quantity: 1, PeriodEnd: f.expires}}
	if err := f.legacy.Reconcile(ctx, legacyEvent.ID, legacyAuthority); err != nil {
		t.Fatal(err)
	}
	testdb.ReconstructCheckout33(t, f.inbox.pool)
	query := `SELECT json_build_object('requests',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM billing_checkout_requests r),'audits',(SELECT jsonb_agg(to_jsonb(e)-'completion_event_id' ORDER BY id) FROM billing_checkout_events e),'notifications',(SELECT jsonb_agg(to_jsonb(n)-'applied_at' ORDER BY event_id) FROM billing_checkout_notifications n),'bindings',(SELECT jsonb_agg(to_jsonb(b)-'checkout_request_id'-'checkout_actor_id'-'completion_event_id' ORDER BY organization_id) FROM billing_customer_bindings b),'observations',(SELECT jsonb_agg(to_jsonb(o)-'completion_event_id'-'checkout_payment_state' ORDER BY organization_id) FROM billing_subscription_observations o),'jobs',(SELECT jsonb_agg(to_jsonb(j) ORDER BY id) FROM river_job j))::text`
	var before, after string
	if err := f.inbox.pool.QueryRow(ctx, query).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrate.Apply(ctx, f.inbox.pool); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.inbox.pool.QueryRow(ctx, query).Scan(&after); err != nil || after != before {
		t.Fatal("additive upgrade changed original evidence/jobs")
	}
	// The additive columns must be null before any mapping; the original identities stay pending.
	var untouched bool
	if err := f.inbox.pool.QueryRow(ctx, `SELECT state='pending' AND applied_at IS NULL AND request_id=$2 AND checkout_id=$3 AND customer_id=$4 AND subscription_id=$5 FROM billing_checkout_notifications WHERE event_id=$1`, e.ID, e.Request, e.Checkout, e.Customer, e.Subscription).Scan(&untouched); err != nil || !untouched || before == "" {
		t.Fatal("migration invented mapping")
	}
	if err := f.inbox.Reconcile(ctx, e.ID, f.intents, f.adapter); err != nil {
		t.Fatal(err)
	}
}
func TestCompletionMappingLegacyScopeAndPriceRemainBound(t *testing.T) {
	for _, mode := range []string{"scope", "price", "owner"} {
		t.Run(mode, func(t *testing.T) {
			f := mappingFixture(t)
			ctx := context.Background()
			e := f.event(t, "evt_mappingBound", "checkout.session.completed")
			if _, err := f.inbox.Receive(ctx, e); err != nil {
				t.Fatal(err)
			}
			if err := f.inbox.Reconcile(ctx, e.ID, f.intents, f.adapter); err != nil {
				t.Fatal(err)
			}
			event := fixtureEvent(t, "evt_mappingLegacyRefused", time.Now().Unix())
			event.Customer = e.Customer
			event.Subscription = e.Subscription
			if _, err := f.legacy.Receive(ctx, event); err != nil {
				t.Fatal(err)
			}
			authority, err := NewAuthority(f.adapter.settings.Billing)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "scope" {
				authority.(*StripeAuthority).origin = "http://127.0.0.1:1"
			}
			if mode == "price" {
				f.mode.Store("price")
			}
			if mode == "owner" {
				_, err = f.inbox.pool.Exec(ctx, `WITH next AS (INSERT INTO owners(issuer,subject,display_name) VALUES('local','changed-legacy-owner','Next') RETURNING id) UPDATE organizations SET owner_id=(SELECT id FROM next) WHERE id=$1`, f.receipt.OrganizationID)
				if err != nil {
					t.Fatal(err)
				}
			}
			before := f.ledger(t)
			calls := f.calls.Load()
			if err = f.legacy.Reconcile(ctx, event.ID, authority); err == nil || f.ledger(t) != before {
				t.Fatal("legacy event escaped original mapping scope")
			}
			want := calls
			if mode == "price" {
				want++
			}
			if f.calls.Load() != want {
				t.Fatal("legacy uncertain scope called authority")
			}
			if mode != "price" && !errors.Is(err, ErrUnavailable) {
				t.Fatal("legacy scope did not fail unavailable")
			}
		})
	}
}

func TestCompletionMappingConcurrentEventsKeepOneBindingAudit(t *testing.T) {
	f := mappingFixture(t)
	ctx := context.Background()
	one := f.event(t, "evt_mappingConcurrentOne", "checkout.session.completed")
	two := f.event(t, "evt_mappingConcurrentTwo", "checkout.session.async_payment_succeeded")
	for _, e := range []CompletionEvent{one, two} {
		if _, err := f.inbox.Receive(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 2)
	for _, e := range []CompletionEvent{one, two} {
		go func(id string) { done <- f.inbox.Reconcile(ctx, id, f.intents, f.adapter) }(e.ID)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var valid bool
	if err := f.inbox.pool.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_customer_bindings) AND (SELECT count(*)=1 FROM billing_checkout_events WHERE action='completion_bound') AND (SELECT count(*)=2 FROM billing_checkout_notifications WHERE state='applied') AND (SELECT count(*)=1 FROM billing_subscription_observations) AND NOT EXISTS(SELECT 1 FROM pilot_entitlement_operations)`).Scan(&valid); err != nil || !valid || f.calls.Load() != 6 {
		t.Fatal("concurrent mapping duplicated custody/audit or granted access")
	}
	before := f.ledger(t)
	calls := f.calls.Load()
	holdBillingFixture(t, f.legacy)
	if err := f.inbox.Reconcile(ctx, one.ID, f.intents, f.adapter); err == nil || f.ledger(t) != before || f.calls.Load() != calls {
		t.Fatal("held applied replay acknowledged authority")
	}
}
