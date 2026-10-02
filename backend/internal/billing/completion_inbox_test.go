package billing

import (
	"bytes"
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"github.com/riverqueue/river"
)

func completionInboxFixture(t *testing.T) (*CompletionInbox, *Store, CompletionEvent, []byte) {
	t.Helper()
	s, pool, _, _, _ := fixtureCheckoutIntents(t)
	s.settings.Billing.WebhookSecret = fixtureSecret(t, "whsec_")
	adapter, err := NewFixtureSessions(s.settings)
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := NewCompletionInbox(true, pool, adapter)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	raw := fixturePayload(t, "evt_inboxCompletion", "checkout.session.completed", time.Now().Unix(), completionFixtureObject())
	e, ok, err := VerifyCompletion(raw, signature(raw, inbox.secret, time.Now()), inbox.secret)
	if err != nil || !ok {
		t.Fatal("invalid completion fixture")
	}
	return inbox, legacy, e, raw
}
func TestCompletionInboxAtomicDuplicatePrivacyAndCrossInbox(t *testing.T) {
	inbox, legacy, e, _ := completionInboxFixture(t)
	ctx := context.Background()
	results := make(chan error, 8)
	for range 8 {
		go func() { _, err := inbox.Receive(ctx, e); results <- err }()
	}
	for range 8 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var safe bool
	err := inbox.pool.QueryRow(ctx, `SELECT (SELECT count(*)=1 FROM billing_checkout_notifications WHERE state='pending') AND (SELECT count(*)=1 FROM river_job WHERE kind='billing_checkout_completion') AND NOT EXISTS(SELECT 1 FROM billing_customer_bindings) AND NOT EXISTS(SELECT 1 FROM billing_subscription_observations) AND NOT EXISTS(SELECT 1 FROM billing_checkout_notifications WHERE row_to_json(billing_checkout_notifications)::text LIKE '%private-marker%')`).Scan(&safe)
	if err != nil || !safe {
		t.Fatal("intake duplicated work, leaked raw content or granted authority", err)
	}
	e.Digest = bytes.Repeat([]byte{1}, 32)
	if dup, err := inbox.Receive(ctx, e); err != nil || !dup {
		t.Fatal("delivery metadata changed immutable identity")
	}
	altered := e
	altered.Checkout = "cs_test_foreign"
	if _, err := inbox.Receive(ctx, altered); !errors.Is(err, ErrConflict) {
		t.Fatal("changed checkout identity accepted")
	}
	subscription := fixtureEvent(t, e.ID, e.Created)
	if _, err := legacy.Receive(ctx, subscription); !errors.Is(err, ErrConflict) {
		t.Fatal("cross inbox collision accepted")
	}
	subscription.ID = "evt_legacyFirst"
	if _, err := legacy.Receive(ctx, subscription); err != nil {
		t.Fatal(err)
	}
	e.ID = subscription.ID
	if _, err := inbox.Receive(ctx, e); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy-first collision accepted")
	}
	if _, err := inbox.pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT reject_completion CHECK(kind<>'billing_checkout_completion') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	e.ID = "evt_rollbackCompletion"
	if _, err := inbox.Receive(ctx, e); err == nil {
		t.Fatal("failed job accepted notification")
	}
	var n int
	if err := inbox.pool.QueryRow(ctx, `SELECT count(*) FROM billing_checkout_notifications`).Scan(&n); err != nil || n != 1 {
		t.Fatal("rollback retained notification")
	}
}
func TestCompletionInboxRecoveryFencesAndPendingWorker(t *testing.T) {
	for _, mode := range []string{"held", "missing", "late_lock"} {
		t.Run(mode, func(t *testing.T) {
			inbox, legacy, e, _ := completionInboxFixture(t)
			ctx := context.Background()
			if mode == "late_lock" {
				lock, err := inbox.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Rollback(ctx)
				if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,73647333))`, e.ID); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { _, err := inbox.Receive(ctx, e); done <- err }()
				waitCheckoutLock(t, inbox.pool, "SELECT pg_advisory_xact_lock(hashtextextended")
				holdBillingFixture(t, legacy)
				if err = lock.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if err = <-done; err == nil {
					t.Fatal("post-lock hold accepted delivery")
				}
			} else {
				if _, err := inbox.Receive(ctx, e); err != nil {
					t.Fatal(err)
				}
				if mode == "held" {
					holdBillingFixture(t, legacy)
				} else if _, err := inbox.pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
					t.Fatal(err)
				}
				if _, err := inbox.Receive(ctx, e); err == nil {
					t.Fatal("held duplicate acknowledged")
				}
				e.ID = "evt_heldNew"
				if _, err := inbox.Receive(ctx, e); err == nil {
					t.Fatal("held new notification accepted")
				}
			}
			var n int
			want := 1
			if mode == "late_lock" {
				want = 0
			}
			if err := inbox.pool.QueryRow(ctx, `SELECT count(*) FROM billing_checkout_notifications`).Scan(&n); err != nil || n != want {
				t.Fatal("held intake changed ledger")
			}
			if err := (&CompletionWorker{}).Work(ctx, &river.Job[CompletionArgs]{Args: CompletionArgs{EventID: e.ID}}); err == nil {
				t.Fatal("pending work acknowledged")
			}
		})
	}
}
func TestCompletionInboxHTTPIndependentGate(t *testing.T) {
	inbox, _, e, raw := completionInboxFixture(t)
	for _, enabled := range []bool{false, true} {
		mux := http.NewServeMux()
		var active *CompletionInbox
		if enabled {
			active = inbox
		}
		active.Register()(mux)
		send := func(body []byte, browser bool) int {
			req := httptest.NewRequest("POST", "/api/billing/checkout-webhook", bytes.NewReader(body))
			req.Header.Set("Stripe-Signature", signature(body, inbox.secret, time.Now()))
			if browser {
				req.Header["Origin"] = []string{""}
			}
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			return res.Code
		}
		if !enabled {
			if send(raw, false) != 503 {
				t.Fatal("nil inbox enabled route")
			}
			continue
		}
		if send(raw, true) != 400 || send(raw, false) != 200 || send(raw, false) != 200 {
			t.Fatal("HTTP gate or durable retry failed")
		}
		e.Request = "bad"
		if _, err := inbox.Receive(context.Background(), e); !errors.Is(err, ErrInvalidEvent) {
			t.Fatal("invalid direct claim accepted")
		}
		unsupported := fixturePayload(t, "evt_completionIgnored", "checkout.session.expired", time.Now().Unix(), completionFixtureObject())
		if send(unsupported, false) != 200 {
			t.Fatal("verified unsupported event not ignored")
		}
		if send(bytes.Repeat([]byte("x"), 262145), false) != 400 {
			t.Fatal("oversized event accepted")
		}
	}
}
func TestCompletionInboxMigrationPreservesPriorLedger(t *testing.T) {
	inbox, legacy, e, _ := completionInboxFixture(t)
	ctx := context.Background()
	prior := fixtureEvent(t, "evt_upgradePrior", e.Created)
	if _, err := legacy.Receive(ctx, prior); err != nil {
		t.Fatal(err)
	}
	var before, after string
	query := `SELECT json_build_object('events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY event_id) FROM billing_events e),'jobs',(SELECT jsonb_agg(to_jsonb(j) ORDER BY id) FROM river_job j))::text`
	if err := inbox.pool.QueryRow(ctx, query).Scan(&before); err != nil {
		t.Fatal(err)
	}
	testdb.ReconstructCheckout33(t, inbox.pool)
	if _, err := inbox.pool.Exec(ctx, `DROP TABLE billing_checkout_notifications; UPDATE schema_version SET version=32`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrate.Apply(ctx, inbox.pool); err != nil {
			t.Fatal(err)
		}
	}
	if err := inbox.pool.QueryRow(ctx, query).Scan(&after); err != nil || after != before {
		t.Fatal("upgrade changed prior billing work")
	}
	if _, err := inbox.Receive(ctx, e); err != nil {
		t.Fatal(err)
	}
}

func TestCompletionInboxCrossTypeRaceAndLocalConstruction(t *testing.T) {
	inbox, legacy, e, _ := completionInboxFixture(t)
	ctx := context.Background()
	subscription := fixtureEvent(t, e.ID, e.Created)
	results := make(chan error, 2)
	go func() { _, err := inbox.Receive(ctx, e); results <- err }()
	go func() { _, err := legacy.Receive(ctx, subscription); results <- err }()
	var accepted, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatal("concurrent cross-type identity accepted twice")
	}
	var n int
	if err := inbox.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM billing_events)+(SELECT count(*) FROM billing_checkout_notifications)`).Scan(&n); err != nil || n != 1 {
		t.Fatal("two inboxes retained one event")
	}
	if s, err := NewCompletionInbox(false, inbox.pool, nil); s != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatal("nonlocal construction accepted")
	}
}
func TestCompletionInboxHoldDuringInsertRollsBack(t *testing.T) {
	inbox, legacy, e, _ := completionInboxFixture(t)
	ctx := context.Background()
	if _, err := inbox.pool.Exec(ctx, `CREATE FUNCTION block_completion_insert() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(42033); RETURN NEW; END$$; CREATE TRIGGER block_completion_insert BEFORE INSERT ON billing_checkout_notifications FOR EACH ROW EXECUTE FUNCTION block_completion_insert()`); err != nil {
		t.Fatal(err)
	}
	lock, err := inbox.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(42033)`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := inbox.Receive(ctx, e); done <- err }()
	waitCheckoutLock(t, inbox.pool, "INSERT INTO billing_checkout_notifications")
	holdBillingFixture(t, legacy)
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("hold during insert accepted completion")
	}
	var empty bool
	if err = inbox.pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM billing_checkout_notifications) AND NOT EXISTS(SELECT 1 FROM river_job WHERE kind='billing_checkout_completion')`).Scan(&empty); err != nil || !empty {
		t.Fatal("held insert retained notification or job")
	}
}
