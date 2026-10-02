package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	stripe "github.com/stripe/stripe-go/v87"
)

func recoverySessionResponse(origin, id, key, customer string, expires time.Time) map[string]any {
	result := sessionResponse(origin, "checkout")
	result["id"], result["client_reference_id"], result["customer"] = id, key, customer
	result["expires_at"], result["url"] = expires.Unix(), origin+"/checkout/"+id
	result["line_items"] = map[string]any{"object": "list", "has_more": false, "data": []any{map[string]any{"object": "item", "quantity": 1, "price": map[string]any{"object": "price", "id": "price_fixtureSessions"}}}}
	return result
}

func TestCheckoutRetrieveUsesReadOnlyPinnedTransport(t *testing.T) {
	for _, status := range []string{"open", "complete", "expired"} {
		t.Run(status, func(t *testing.T) {
			var origin string
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			var calls atomic.Int32
			adapter, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/v1/checkout/sessions/cs_test_fixtureSessions" || r.URL.Query().Get("expand[0]") != "line_items" || len(r.URL.Query()) != 1 || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("Stripe-Version") != stripe.APIVersion {
					t.Error("retrieval escaped exact read-only SDK request")
				}
				body := recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
				body["status"] = status
				if status != "open" {
					body["url"] = "https://untrusted.test/closed-private-marker"
				}
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error("fixture encoding failed")
				}
			})
			origin = server.URL
			for range 2 {
				result, err := adapter.RetrieveCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", expires)
				if err != nil || result.Status != status || !result.ExpiresAt.Equal(expires) || (status == "open") != (result.URL != "") {
					t.Fatal("confirmed session read failed or disclosed closed navigation")
				}
			}
			if calls.Load() != 2 {
				t.Fatal("retrieval created or retried another resource")
			}
		})
	}
}

func TestCheckoutRetrieveRejectsUnconfirmedRemoteShape(t *testing.T) {
	for _, mode := range []string{"customer", "key", "id", "live", "object", "mode", "ui", "success", "cancel", "expiry", "items_missing", "items_more", "items_many", "item_nil", "price", "quantity", "status", "url", "redirect", "error", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			var origin string
			var calls atomic.Int32
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			adapter, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body := recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
				switch mode {
				case "customer":
					body["customer"] = "cus_foreign"
				case "key":
					body["client_reference_id"] = "foreign"
				case "id":
					body["id"] = "cs_test_other"
				case "live":
					body["livemode"] = true
				case "object":
					body["object"] = "foreign"
				case "mode":
					body["mode"] = "payment"
				case "ui":
					body["ui_mode"] = "embedded_page"
				case "success":
					body["success_url"] = "https://foreign.test"
				case "cancel":
					body["cancel_url"] = "https://foreign.test"
				case "expiry":
					body["expires_at"] = expires.Add(time.Minute).Unix()
				case "items_missing":
					body["line_items"] = nil
				case "items_more":
					body["line_items"].(map[string]any)["has_more"] = true
				case "items_many":
					body["line_items"].(map[string]any)["data"] = []any{nil, nil}
				case "item_nil":
					body["line_items"].(map[string]any)["data"] = []any{nil}
				case "price":
					body["line_items"].(map[string]any)["data"].([]any)[0].(map[string]any)["price"] = nil
				case "quantity":
					body["line_items"].(map[string]any)["data"].([]any)[0].(map[string]any)["quantity"] = 2
				case "status":
					body["status"] = "unknown"
				case "url":
					body["url"] = origin + "/checkout/cs_test_fixtureSessions?private=marker"
				case "redirect":
					http.Redirect(w, r, "http://127.0.0.1:1/private", 307)
					return
				case "error":
					w.WriteHeader(500)
					return
				case "malformed":
					_, _ = w.Write([]byte("{invalid"))
					return
				}
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error("fixture encoding failed")
				}
			})
			origin = server.URL
			result, err := adapter.RetrieveCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", expires)
			if !errors.Is(err, ErrUnavailable) || result != (RecoveredCheckout{}) || calls.Load() != 1 {
				t.Fatal("unconfirmed session escaped or request repeated")
			}
		})
	}
}

func TestCheckoutRetrieveInputAndCancellationBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, input := range []struct {
		request SessionRequest
		id      string
		expires time.Time
	}{
		{SessionRequest{}, "cs_test_fixtureSessions", time.Now()},
		{sessionFixtureRequest, "cs_test_x/path", time.Now()},
		{sessionFixtureRequest, "cs_test_fixtureSessions", time.Time{}},
	} {
		if _, err := adapter.RetrieveCheckout(context.Background(), input.request, input.id, input.expires); !errors.Is(err, ErrInvalidEvent) {
			t.Fatal("invalid input accepted")
		}
	}
	var absent *FixtureSessions
	if _, err := absent.RetrieveCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", time.Now()); !errors.Is(err, ErrInvalidEvent) {
		t.Fatal("absent adapter accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.RetrieveCheckout(ctx, sessionFixtureRequest, "cs_test_fixtureSessions", time.Now().Add(time.Hour)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled retrieval returned result")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid or cancelled input reached authority")
	}
}

func readyCheckoutRecovery(t *testing.T, hook func()) (*CheckoutIntents, CheckoutIntent, string, *FixtureSessions, *atomic.Int32) {
	t.Helper()
	_, pool, org := fixtureStore(t)
	ctx := context.Background()
	var owner string
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var origin, requestID string
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	calls := new(atomic.Int32)
	adapter, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" {
			t.Error("session recovery mutated authority")
			w.WriteHeader(400)
			return
		}
		if hook != nil {
			hook()
		}
		if err := json.NewEncoder(w).Encode(recoverySessionResponse(origin, "cs_test_recoveryFixture", requestID, "cus_recoveryFixture", expires)); err != nil {
			t.Error("fixture encoding failed")
		}
	})
	origin = server.URL
	s, err := NewCheckoutIntents(true, pool, adapter)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(ctx, owner, org, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.reserveDispatch(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE billing_checkout_requests SET state='session_ready',customer_id='cus_recoveryFixture',checkout_id='cs_test_recoveryFixture',session_expires_at=$2 WHERE id=$1`, p.ID, expires); err != nil {
		t.Fatal(err)
	}
	requestID = p.ID
	return s, p, owner, adapter, calls
}

func TestCheckoutSessionRecoveryPreservesWorkAndSuppressesExpiredRPC(t *testing.T) {
	s, p, owner, a, calls := readyCheckoutRecovery(t, nil)
	ctx := context.Background()
	before := checkoutLedger(t, s)
	for range 2 {
		got, err := s.RecoverSession(ctx, owner, p.OrganizationID, p.ID, a)
		if err != nil || got.Status != "open" || got.URL == "" {
			t.Fatal("original session navigation unavailable")
		}
	}
	if checkoutLedger(t, s) != before || calls.Load() != 2 {
		t.Fatal("read changed checkout work")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE billing_checkout_requests SET session_expires_at=clock_timestamp()-interval '1 hour' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	before = checkoutLedger(t, s)
	got, err := s.RecoverSession(ctx, owner, p.OrganizationID, p.ID, a)
	if err != nil || got.Status != "expired" || got.URL != "" || calls.Load() != 2 || checkoutLedger(t, s) != before {
		t.Fatal("expired checkpoint recreated or changed checkout")
	}
}

func TestCheckoutSessionRecoveryRefusesChangedScopeBeforeRPC(t *testing.T) {
	for _, mode := range []string{"actor", "org", "owner", "price", "authority", "credential", "adapter", "binding", "unfinished", "held", "missing"} {
		t.Run(mode, func(t *testing.T) {
			s, p, owner, a, calls := readyCheckoutRecovery(t, nil)
			ctx := context.Background()
			org := p.OrganizationID
			var err error
			switch mode {
			case "actor":
				owner = sessionFixtureRequest.Key
			case "org":
				org = sessionFixtureRequest.Key
			case "owner":
				var next string
				err = s.pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','recover-new','New') RETURNING id`).Scan(&next)
				if err == nil {
					_, err = s.pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, next, org)
				}
			case "price":
				s.settings.PriceID = "price_changed"
				a.settings.PriceID = s.settings.PriceID
			case "authority":
				s.settings.Billing.FixtureURL = "http://127.0.0.1:5559"
				a.settings.Billing.FixtureURL = s.settings.Billing.FixtureURL
			case "credential":
				s.keyDigest[0] ^= 1
				a.keyDigest = s.keyDigest
			case "adapter":
				a = nil
			case "binding":
				_, err = s.pool.Exec(ctx, `INSERT INTO billing_customer_bindings(organization_id,customer_id,subscription_id,idempotency_key,actor,reason) VALUES($1,'cus_other','sub_other',$2,'fixture','qualification')`, org, p.Key)
			case "unfinished":
				_, err = s.pool.Exec(ctx, `UPDATE billing_checkout_requests SET state='customer_ready',checkout_id=NULL,session_expires_at=NULL WHERE id=$1`, p.ID)
			case "held":
				_, err = s.pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32))
			case "missing":
				_, err = s.pool.Exec(ctx, `DROP TABLE management_recovery`)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := checkoutLedger(t, s)
			got, err := s.RecoverSession(ctx, owner, org, p.ID, a)
			if err == nil || got != (RecoveredCheckout{}) || calls.Load() != 0 || checkoutLedger(t, s) != before {
				t.Fatal("changed scope disclosed navigation or mutated work")
			}
		})
	}
}

func TestCheckoutSessionRecoveryHoldDuringReadDiscardsNavigation(t *testing.T) {
	var s *CheckoutIntents
	hook := func() {
		if _, err := s.pool.Exec(context.Background(), `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
			t.Error("fixture hold failed")
		}
	}
	store, p, owner, a, calls := readyCheckoutRecovery(t, hook)
	s = store
	before := checkoutLedger(t, s)
	got, err := s.RecoverSession(context.Background(), owner, p.OrganizationID, p.ID, a)
	if !errors.Is(err, recovery.ErrHeld) || got != (RecoveredCheckout{}) || calls.Load() != 1 || checkoutLedger(t, s) != before {
		t.Fatal("hold disclosed navigation or changed retained work")
	}
}

func TestCheckoutSessionRecoveryRechecksAfterCustodyWait(t *testing.T) {
	s, p, owner, a, calls := readyCheckoutRecovery(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, p.OrganizationID); err != nil {
		t.Fatal(err)
	}
	before := checkoutLedger(t, s)
	done := make(chan error, 1)
	go func() { _, err := s.RecoverSession(ctx, owner, p.OrganizationID, p.ID, a); done <- err }()
	waitCheckoutLock(t, s.pool, "SELECT id FROM organizations")
	if _, err = s.pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, recovery.ErrHeld) || calls.Load() != 0 || checkoutLedger(t, s) != before {
		t.Fatal("custody wait bypassed recovery fence")
	}
}

func TestCheckoutSessionRecoveryRechecksAfterRequestWait(t *testing.T) {
	s, p, owner, a, calls := readyCheckoutRecovery(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM billing_checkout_requests WHERE id=$1 FOR UPDATE`, p.ID); err != nil {
		t.Fatal(err)
	}
	before := checkoutLedger(t, s)
	done := make(chan error, 1)
	go func() { _, err := s.RecoverSession(ctx, owner, p.OrganizationID, p.ID, a); done <- err }()
	waitCheckoutLock(t, s.pool, "SELECT actor_id,price_id,return_origin")
	if _, err = s.pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, recovery.ErrHeld) || calls.Load() != 0 || checkoutLedger(t, s) != before {
		t.Fatal("request wait bypassed recovery fence")
	}
}

func TestCheckoutRetrievePastOpenSessionHasNoNavigation(t *testing.T) {
	var origin string
	expires := time.Now().Add(-time.Minute).Truncate(time.Second)
	a, server := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		body := recoverySessionResponse(origin, "cs_test_fixtureSessions", sessionFixtureRequest.Key, sessionFixtureRequest.CustomerID, expires)
		body["url"] = "https://foreign.test/private"
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error("fixture encoding failed")
		}
	})
	origin = server.URL
	result, err := a.RetrieveCheckout(context.Background(), sessionFixtureRequest, "cs_test_fixtureSessions", expires)
	if err != nil || result.Status != "expired" || result.URL != "" {
		t.Fatal("past open session disclosed navigation")
	}
}
