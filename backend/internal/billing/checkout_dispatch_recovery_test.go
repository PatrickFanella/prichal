package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

func checkoutLedger(t *testing.T, s *CheckoutIntents) string {
	t.Helper()
	var rows string
	if err := s.pool.QueryRow(context.Background(), `SELECT json_build_object('requests',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM billing_checkout_requests x),'audits',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM billing_checkout_events x),'jobs',(SELECT json_agg(row_to_json(x) ORDER BY id) FROM river_job x))::text`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCheckoutRecoveryBeforeDispatchAndDuringCustodyWait(t *testing.T) {
	for _, wait := range []bool{false, true} {
		for _, ledger := range []string{"held", "missing"} {
			t.Run(ledger+map[bool]string{true: "/wait", false: "/before"}[wait], func(t *testing.T) {
				s, p, owner, m := preparedCheckout(t)
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
					t.Fatal(err)
				}
				before := checkoutLedger(t, s)
				tx, err := s.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				if wait {
					if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, p.OrganizationID); err != nil {
						t.Fatal(err)
					}
				}
				done := make(chan error, 1)
				if wait {
					go func() { done <- s.Dispatch(ctx, p.ID, m) }()
					waitCheckoutLock(t, s.pool, "SELECT id FROM organizations")
				}
				if ledger == "held" {
					_, err = s.pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32))
				} else {
					_, err = s.pool.Exec(ctx, `DROP TABLE management_recovery`)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if wait {
					err = <-done
				} else {
					err = s.Dispatch(ctx, p.ID, m)
				}
				if err == nil || ledger == "held" && !errors.Is(err, recovery.ErrHeld) {
					t.Fatal("uncertain recovery allowed checkout dispatch")
				}
				if c, k := m.counts(); c != 0 || k != 0 || checkoutLedger(t, s) != before {
					t.Fatal("held checkout changed ledger or called authority")
				}
			})
		}
	}
}

func TestCheckoutHoldDuringRemoteResponsePreservesPendingCheckpoint(t *testing.T) {
	for _, stage := range []string{"customer", "session"} {
		t.Run(stage, func(t *testing.T) {
			s, p, owner, m := preparedCheckout(t)
			ctx := context.Background()
			if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
				t.Fatal(err)
			}
			if err := s.reserveDispatch(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
			if stage == "session" {
				tx, actor, org, err := s.dispatchScope(ctx, p.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.dispatchStep(ctx, tx, p.ID, actor, org, m); err != nil {
					t.Fatal(err)
				}
			}
			before := checkoutLedger(t, s)
			c0, k0 := m.counts()
			hook := func() {
				if _, err := s.pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
					t.Error("fixture hold failed")
				}
			}
			if stage == "customer" {
				m.onCustomer = hook
			} else {
				m.onSession = hook
			}
			if err := s.Dispatch(ctx, p.ID, m); !errors.Is(err, recovery.ErrHeld) {
				t.Fatal("held remote result committed")
			}
			c, k := m.counts()
			if stage == "customer" && (c != c0+1 || k != k0) || stage == "session" && (c != c0 || k != k0+1) {
				t.Fatal("hold dispatched another remote step")
			}
			if checkoutLedger(t, s) != before {
				t.Fatal("held remote response changed pending checkpoint")
			}
		})
	}
}

func TestCheckoutChangedCustodyConfigurationAndFutureClockRefuseDispatch(t *testing.T) {
	for _, mode := range []string{"owner", "configuration", "authority", "credential", "future", "binding"} {
		t.Run(mode, func(t *testing.T) {
			s, p, owner, m := preparedCheckout(t)
			ctx := context.Background()
			if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
				t.Fatal(err)
			}
			if err := s.reserveDispatch(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			switch mode {
			case "owner":
				var next string
				if err = s.pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','dispatch-foreign','Foreign') RETURNING id`).Scan(&next); err == nil {
					_, err = s.pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, next, p.OrganizationID)
				}
			case "configuration":
				s.settings.PriceID = "price_changed"
			case "authority":
				s.settings.Billing.FixtureURL = "http://127.0.0.1:5561"
			case "credential":
				s.keyDigest[0] ^= 1
			case "future":
				_, err = s.pool.Exec(ctx, `UPDATE billing_checkout_requests SET dispatch_started_at=clock_timestamp()+interval '2 hours' WHERE id=$1`, p.ID)
			case "binding":
				_, err = s.pool.Exec(ctx, `INSERT INTO billing_customer_bindings(organization_id,customer_id,subscription_id,idempotency_key,actor,reason) VALUES($1,'cus_changed','sub_changed',$2,'fixture','qualification')`, p.OrganizationID, p.Key)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := checkoutLedger(t, s)
			if err = s.Dispatch(ctx, p.ID, m); err == nil {
				t.Fatal("changed authority dispatched checkout")
			}
			if c, k := m.counts(); c != 0 || k != 0 || checkoutLedger(t, s) != before {
				t.Fatal("uncertain authority changed checkout work")
			}
		})
	}
}

func TestCheckoutSessionSettingsStaySeparateFromNotificationFixture(t *testing.T) {
	adapter, _ := fixtureSessionAdapter(t, nil)
	s := adapter.settings
	t.Setenv("PDS_BILLING_SESSIONS_MODE", "")
	if loaded, err := LoadSessionSettings(true, s.Billing); err != nil || loaded.Billing.Enabled {
		t.Fatal("notification fixture opened checkout")
	}
	t.Setenv("PDS_BILLING_SESSIONS_MODE", "fixture")
	t.Setenv("PDS_BILLING_PRICE_ID", s.PriceID)
	t.Setenv("PDS_BILLING_PORTAL_CONFIGURATION_ID", s.PortalConfigurationID)
	t.Setenv("PDS_BILLING_RETURN_ORIGIN", s.ReturnOrigin)
	if loaded, err := LoadSessionSettings(true, s.Billing); err != nil || !loaded.Billing.Enabled {
		t.Fatal("explicit complete session fixture was not loaded")
	}
	if _, err := LoadSessionSettings(false, s.Billing); err != ErrUnavailable {
		t.Fatal("production checkout fixture enabled")
	}
	if _, err := LoadSessionSettings(true, Settings{}); err != ErrUnavailable {
		t.Fatal("checkout enabled without fixture authority")
	}
	t.Setenv("PDS_BILLING_SESSIONS_MODE", "live")
	if _, err := LoadSessionSettings(true, s.Billing); err != ErrUnavailable {
		t.Fatal("live checkout mode enabled")
	}
}

func TestCheckoutHoldDuringQueueAuditRollsBackStart(t *testing.T) {
	s, p, owner, _ := preparedCheckout(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	before := checkoutLedger(t, s)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `LOCK TABLE billing_checkout_events IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	type answer struct {
		r   CheckoutIntent
		err error
	}
	done := make(chan answer, 1)
	go func() { r, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); done <- answer{r, err} }()
	waitCheckoutLock(t, s.pool, "INSERT INTO billing_checkout_events")
	if _, err = s.pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a := <-done
	if !errors.Is(a.err, recovery.ErrHeld) || a.r.ID != "" || checkoutLedger(t, s) != before {
		t.Fatal("hold during queue audit committed dispatch work")
	}
}

func TestCheckoutCustomerCannotBeMappedIntoTwoOrganizations(t *testing.T) {
	s, p, owner, m := preparedCheckout(t)
	ctx := context.Background()
	if _, err := s.Start(ctx, owner, p.OrganizationID, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(ctx, p.ID, m); err != nil {
		t.Fatal(err)
	}
	var other string
	if err := s.pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,'Other checkout fixture') RETURNING id`, owner).Scan(&other); err != nil {
		t.Fatal(err)
	}
	second, err := s.Prepare(ctx, owner, other, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Start(ctx, owner, other, second.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = s.Dispatch(ctx, second.ID, m); err == nil {
		t.Fatal("customer shared across organization custody")
	}
	r, err := s.Get(ctx, owner, other, second.ID)
	if err != nil || r.State != "queued" {
		t.Fatal("customer collision advanced second preparation")
	}
	var customers int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM billing_checkout_requests WHERE customer_id='cus_dispatchFixture'`).Scan(&customers); err != nil || customers != 1 {
		t.Fatal("customer collision overwrote first mapping")
	}
	if c, k := m.counts(); c != 2 || k != 1 {
		t.Fatal("customer collision created competing checkout")
	}
}
