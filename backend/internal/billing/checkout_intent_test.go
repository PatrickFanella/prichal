package billing

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCheckoutPreparationUpgradePreservesVersionThirtyBilling(t *testing.T) {
	store, pool, org := fixtureStore(t)
	ctx := context.Background()
	bindFixture(t, store, org)
	event := fixtureEvent(t, "evt_checkoutUpgrade", time.Now().Unix())
	if _, err := store.Receive(ctx, event); err != nil {
		t.Fatal(err)
	}
	query := `SELECT json_build_object('owners',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM owners x),'organizations',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM organizations x),'bindings',(SELECT json_agg(row_to_json(x) ORDER BY organization_id) FROM billing_customer_bindings x),'events',(SELECT json_agg(row_to_json(x) ORDER BY event_id) FROM billing_events x),'jobs',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM river_job x))::text`
	var before, after string
	if err := pool.QueryRow(ctx, query).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DROP TABLE billing_checkout_events,billing_checkout_requests; UPDATE schema_version SET version=30`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrate.Apply(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, query).Scan(&after); err != nil || after != before {
		t.Fatal("checkout upgrade changed retained billing, custody or queued work")
	}
	if requests, events, _ := checkoutCounts(t, pool); requests != 0 || events != 0 {
		t.Fatal("checkout upgrade invented intent")
	}
	if err := (migrate.Probe{Pool: pool}).Check(ctx); err != nil {
		t.Fatal("checkout upgrade schema is not ready")
	}
}

func waitCheckoutLock(t *testing.T, pool *pgxpool.Pool, query string) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		var waiting bool
		if err := pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1)`, "%"+query+"%").Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("checkout operation did not reach the expected lock")
}

func TestCheckoutRecoveryIntroducedDuringWaitRefusesResultAndCommit(t *testing.T) {
	for _, mode := range []string{"prepare_org", "retry_org", "read_table", "audit"} {
		for _, ledger := range []string{"held", "missing"} {
			t.Run(mode+"/"+ledger, func(t *testing.T) {
				s, pool, owner, org, calls := fixtureCheckoutIntents(t)
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				var original CheckoutIntent
				var err error
				if mode == "retry_org" || mode == "read_table" {
					original, err = s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
					if err != nil {
						t.Fatal(err)
					}
				}
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				query := "SELECT id FROM organizations"
				switch mode {
				case "prepare_org", "retry_org":
					_, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org)
				case "read_table":
					_, err = tx.Exec(ctx, `LOCK TABLE billing_checkout_requests IN ACCESS EXCLUSIVE MODE`)
					query = "FROM billing_checkout_requests WHERE id="
				case "audit":
					_, err = tx.Exec(ctx, `LOCK TABLE billing_checkout_events IN ACCESS EXCLUSIVE MODE`)
					query = "INSERT INTO billing_checkout_events"
				}
				if err != nil {
					t.Fatal(err)
				}
				type answer struct {
					result CheckoutIntent
					err    error
				}
				done := make(chan answer, 1)
				go func() {
					var a answer
					if mode == "read_table" {
						a.result, a.err = s.Get(ctx, owner, org, original.ID)
					} else {
						a.result, a.err = s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
					}
					done <- a
				}()
				waitCheckoutLock(t, pool, query)
				if ledger == "held" {
					_, err = pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32))
				} else {
					_, err = pool.Exec(ctx, `DROP TABLE management_recovery`)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				a := <-done
				if a.err == nil || a.result.ID != "" {
					t.Fatal("recovery introduced during wait escaped result or commit")
				}
				if ledger == "held" && !errors.Is(a.err, recovery.ErrHeld) {
					t.Fatal("late hold was not preserved")
				}
				want := 0
				if original.ID != "" {
					want = 1
				}
				if requests, events, jobs := checkoutCounts(t, pool); requests != want || events != want || jobs != 0 || calls.Load() != 0 {
					t.Fatal("late recovery changed checkout work")
				}
			})
		}
	}
}

func fixtureCheckoutIntents(t *testing.T) (*CheckoutIntents, *pgxpool.Pool, string, string, *atomic.Int32) {
	t.Helper()
	_, pool, org := fixtureStore(t)
	var owner string
	if err := pool.QueryRow(context.Background(), `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	calls := new(atomic.Int32)
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	s, err := NewCheckoutIntents(true, pool, adapter)
	if err != nil {
		t.Fatal("checkout preparation setup failed")
	}
	return s, pool, owner, org, calls
}

func TestCheckoutPreparationRequiresLocalPolicyAndConfirmedScope(t *testing.T) {
	s, pool, owner, org, calls := fixtureCheckoutIntents(t)
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) })
	for _, input := range []struct {
		local   bool
		pool    *pgxpool.Pool
		adapter *FixtureSessions
	}{{false, pool, adapter}, {true, nil, adapter}, {true, pool, nil}} {
		if store, err := NewCheckoutIntents(input.local, input.pool, input.adapter); err != ErrUnavailable || store != nil {
			t.Fatal("disabled checkout preparation policy accepted")
		}
	}
	ctx := context.Background()
	if _, err := s.GetByKey(ctx, owner, org, "invalid"); err != ErrCheckoutNotFound {
		t.Fatal("invalid receipt key accepted")
	}
	pool.Close()
	if result, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true); err == nil || result.ID != "" {
		t.Fatal("closed database allowed checkout preparation")
	}
	if result, err := s.GetByKey(ctx, owner, org, sessionFixtureRequest.Key); err == nil || result.ID != "" {
		t.Fatal("closed database disclosed checkout receipt")
	}
	if calls.Load() != 0 {
		t.Fatal("unconfirmed checkout preparation dispatched")
	}
}

func checkoutCounts(t *testing.T, pool *pgxpool.Pool) (int, int, int) {
	t.Helper()
	var requests, events, jobs int
	err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM billing_checkout_requests),(SELECT count(*) FROM billing_checkout_events),(SELECT count(*) FROM river_job)`).Scan(&requests, &events, &jobs)
	if err != nil {
		t.Fatal(err)
	}
	return requests, events, jobs
}

func TestCheckoutPreparationConcurrentReplayIsAuditedWithoutDispatch(t *testing.T) {
	s, pool, owner, org, calls := fixtureCheckoutIntents(t)
	ctx := context.Background()
	results := make(chan CheckoutIntent, 8)
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			r, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
			results <- r
			errs <- err
		}()
	}
	var first CheckoutIntent
	for range 8 {
		r := <-results
		if err := <-errs; err != nil {
			t.Fatal("concurrent original preparation failed")
		}
		if first.ID == "" {
			first = r
		}
		if r != first || r.CanPay || r.State != "prepared" {
			t.Fatal("concurrent preparation changed receipt or granted payment")
		}
	}
	got, err := s.Get(ctx, owner, org, first.ID)
	if err != nil || got != first {
		t.Fatal("exact original preparation not readable")
	}
	if receipt, err := s.GetByKey(ctx, owner, org, sessionFixtureRequest.Key); err != nil || receipt != first || receipt.Key != sessionFixtureRequest.Key {
		t.Fatal("lost preparation response did not recover by exact key")
	}
	if requests, events, jobs := checkoutCounts(t, pool); requests != 1 || events != 1 || jobs != 0 || calls.Load() != 0 {
		t.Fatal("preparation duplicated audit or dispatched work")
	}
	if _, err = s.Prepare(ctx, owner, org, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", true); err != ErrConflict {
		t.Fatal("new key replaced retained preparation")
	}
	changed := *s
	changed.settings.PriceID = "price_changed"
	if _, err = changed.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true); err != ErrConflict {
		t.Fatal("retry changed configured plan")
	}
	changed = *s
	changed.settings.ReturnOrigin = "http://127.0.0.1:5174"
	if _, err = changed.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true); err != ErrConflict {
		t.Fatal("retry changed return configuration")
	}
}

func TestCheckoutPreparationConfirmsOriginalAndCurrentOwner(t *testing.T) {
	s, pool, owner, org, _ := fixtureCheckoutIntents(t)
	ctx := context.Background()
	var foreign, otherOrg string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','foreign-checkout','Foreign') RETURNING id`).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,'Other') RETURNING id`, foreign).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, false); err != ErrInvalidEvent {
		t.Fatal("unconfirmed plan prepared")
	}
	if _, err := s.Prepare(ctx, owner, org, "invalid", true); err != ErrInvalidEvent {
		t.Fatal("invalid key prepared")
	}
	if _, err := s.Prepare(ctx, foreign, org, sessionFixtureRequest.Key, true); err != ErrCheckoutNotFound {
		t.Fatal("foreign owner prepared checkout")
	}
	result, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range [][2]string{{foreign, org}, {foreign, otherOrg}, {owner, otherOrg}, {"invalid", org}} {
		if _, err = s.Get(ctx, scope[0], scope[1], result.ID); err != ErrCheckoutNotFound {
			t.Fatal("foreign checkout receipt disclosed")
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, foreign, org); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, owner, org, result.ID); err != ErrCheckoutNotFound {
		t.Fatal("former owner read retained checkout")
	}
	if _, err = s.Get(ctx, foreign, org, result.ID); err != ErrCheckoutNotFound {
		t.Fatal("new owner inherited original checkout")
	}
	if _, err = s.Prepare(ctx, foreign, org, sessionFixtureRequest.Key, true); err != ErrCheckoutNotFound {
		t.Fatal("new owner adopted original key")
	}
	if _, err = s.Prepare(ctx, foreign, org, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", true); err != ErrConflict {
		t.Fatal("new owner replaced unresolved original checkout")
	}
}

func TestCheckoutPreparationAuditAndRecoveryFailureRollBack(t *testing.T) {
	for _, mode := range []string{"audit", "held", "missing"} {
		t.Run(mode, func(t *testing.T) {
			s, pool, owner, org, calls := fixtureCheckoutIntents(t)
			ctx := context.Background()
			var err error
			switch mode {
			case "audit":
				_, err = pool.Exec(ctx, `ALTER TABLE billing_checkout_events ADD CONSTRAINT reject_fixture CHECK(false) NOT VALID`)
			case "held":
				_, err = pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32))
			case "missing":
				_, err = pool.Exec(ctx, `DROP TABLE management_recovery`)
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
			if err == nil || result.ID != "" {
				t.Fatal("unconfirmed preparation escaped")
			}
			if mode == "held" && !errors.Is(err, recovery.ErrHeld) {
				t.Fatal("hold not preserved")
			}
			if requests, events, jobs := checkoutCounts(t, pool); requests != 0 || events != 0 || jobs != 0 || calls.Load() != 0 {
				t.Fatal("failed preparation left work")
			}
		})
	}
}

func TestCheckoutPreparationExistingBillingAndPendingHandoverRefuse(t *testing.T) {
	for _, mode := range []string{"binding", "handover"} {
		t.Run(mode, func(t *testing.T) {
			s, pool, owner, org, _ := fixtureCheckoutIntents(t)
			ctx := context.Background()
			var err error
			if mode == "binding" {
				_, err = pool.Exec(ctx, `INSERT INTO billing_customer_bindings(organization_id,customer_id,subscription_id,idempotency_key,actor,reason) VALUES($1,'cus_bound','sub_bound',$2,'fixture','qualification')`, org, sessionFixtureRequest.Key)
			} else {
				var next string
				if err = pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','handover-checkout','Next') RETURNING id`).Scan(&next); err == nil {
					_, err = pool.Exec(ctx, `INSERT INTO organization_transfers(organization_id,previous_owner_id,next_owner_id,idempotency_key) VALUES($1,$2,$3,$4)`, org, owner, next, sessionFixtureRequest.Key)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true); err != ErrConflict {
				t.Fatal("checkout bypassed retained billing or handover")
			}
		})
	}
}

func TestCheckoutPreparationReadAndRetryRecoveryRefusal(t *testing.T) {
	for _, mode := range []string{"held", "missing"} {
		t.Run(mode, func(t *testing.T) {
			s, pool, owner, org, _ := fixtureCheckoutIntents(t)
			ctx := context.Background()
			original, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "held" {
				_, err = pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32))
			} else {
				_, err = pool.Exec(ctx, `DROP TABLE management_recovery`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result, err := s.Get(ctx, owner, org, original.ID); err == nil || result.ID != "" {
				t.Fatal("held receipt disclosed")
			}
			if result, err := s.GetByKey(ctx, owner, org, sessionFixtureRequest.Key); err == nil || result.ID != "" {
				t.Fatal("held exact-key receipt disclosed")
			}
			if result, err := s.Prepare(ctx, owner, org, sessionFixtureRequest.Key, true); err == nil || result.ID != "" {
				t.Fatal("held retry disclosed")
			}
			if requests, events, jobs := checkoutCounts(t, pool); requests != 1 || events != 1 || jobs != 0 {
				t.Fatal("held retry changed ledger")
			}
		})
	}
}
