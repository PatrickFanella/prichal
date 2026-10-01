package support

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

func TestActivityRefusesUncertainRecovery(t *testing.T) {
	for _, mode := range []string{"held", "missing-ledger"} {
		t.Run(mode, func(t *testing.T) {
			s, actions := fixture(t)
			query := `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`
			if mode == "missing-ledger" {
				query = `DROP TABLE management_recovery`
			}
			if _, err := s.Pool.Exec(context.Background(), query); err != nil {
				t.Fatal(err)
			}
			for _, operator := range []bool{false, true} {
				actor := bob
				if operator {
					actor = alice
				}
				body, err := s.Activity(context.Background(), actor, instance, operator, 0)
				if err == nil || len(body) != 0 {
					t.Fatal("uncertain recovery disclosed activity")
				}
				if mode == "held" && !errors.Is(err, recovery.ErrHeld) {
					t.Fatal(err)
				}
			}
			mux := http.NewServeMux()
			s.Register(sessionFixture{owner: bob}, actions)(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/deployments/"+instance+"/activity", nil))
			if response.Code != 503 {
				t.Fatal("owner activity ignored recovery hold", response.Code)
			}
		})
	}
}

func TestActivitySerializesCustodyAndRechecksHoldAfterPageWait(t *testing.T) {
	for _, mode := range []string{"owner", "held"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			if _, err := s.Pool.Exec(ctx, `INSERT INTO service_operations(deployment_id,idempotency_key,generation,kind,desired_state,prior_state,reason,actor,state) VALUES($1,gen_random_uuid(),1,'service','running','running','maintenance','PRIVATE_OPERATOR','applied')`, instance); err != nil {
				t.Fatal(err)
			}
			block, err := s.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer block.Rollback(context.Background())
			if _, err := block.Exec(ctx, `LOCK TABLE service_operations IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}
			type outcome struct {
				body []byte
				err  error
			}
			done := make(chan outcome, 1)
			go func() { body, err := s.Activity(ctx, bob, instance, false, 0); done <- outcome{body, err} }()
			for {
				var waiting bool
				if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'WITH page AS%')`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("activity never reached blocked page read")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if mode == "owner" {
				change, stop := context.WithTimeout(ctx, 100*time.Millisecond)
				_, err := s.Pool.Exec(change, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, alice, organization)
				stop()
				if err == nil {
					t.Fatal("owner custody changed while scoped activity was being read")
				}
			} else if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
				t.Fatal(err)
			}
			if err := block.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			var result outcome
			select {
			case result = <-done:
			case <-ctx.Done():
				t.Fatal("activity did not complete")
			}
			if mode == "held" {
				if !errors.Is(result.err, recovery.ErrHeld) || len(result.body) != 0 {
					t.Fatal("hold during page lookup disclosed activity", result.err)
				}
			} else {
				if result.err != nil || len(result.body) == 0 {
					t.Fatal("serialized owner read failed", result.err)
				}
				if _, err := s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, alice, organization); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Activity(ctx, bob, instance, false, 0); !errors.Is(err, ErrNotFound) {
					t.Fatal("former owner retained activity access", err)
				}
			}
		})
	}
}

func TestActivityRejectsAmbiguousCursors(t *testing.T) {
	s, actions := fixture(t)
	mux := http.NewServeMux()
	s.Register(sessionFixture{owner: bob}, actions)(mux)
	for _, query := range []string{"?before=1&before=2", "?unknown=1", "?before=", "?before=%zz", "?before=9007199254740992"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/deployments/"+instance+"/activity"+query, nil))
		if response.Code != 400 {
			t.Fatalf("ambiguous cursor accepted: %d", response.Code)
		}
	}
}
