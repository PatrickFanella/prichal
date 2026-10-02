package support

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

func TestOriginalOperatorCanReadExactLifecycleRequestReceipt(t *testing.T) {
	s, actions := fixture(t)
	key := "66666666-6666-4666-8666-666666666666"
	op, err := actions.Submit(context.Background(), "oidc-owner:"+alice, instance, lifecycle.Request{Key: key, Kind: "account", State: "suspended", Reason: "safety_review"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Register(sessionFixture{owner: alice}, actions)(mux)
	path := "/api/operator/deployments/" + instance + "/lifecycle-requests/" + key
	for i := 0; i < 2; i++ {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 {
			t.Fatalf("exact lifecycle receipt unavailable: %d", response.Code)
		}
		var receipt struct {
			ID, DeploymentID, IdempotencyKey, State string
			Generation                              int64
		}
		if json.Unmarshal(response.Body.Bytes(), &receipt) != nil || receipt.ID != op.ID || receipt.DeploymentID != instance || receipt.IdempotencyKey != key || receipt.State != "pending" || receipt.Generation != 1 {
			t.Fatal("receipt scope or authority changed")
		}
		if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "PRIVATE") || strings.Contains(response.Body.String(), "oidc-owner:") {
			t.Fatal("receipt leaked private inspection data")
		}
	}
	var inspections, operations, jobs int
	if err := s.Pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM support_inspections),(SELECT count(*) FROM service_operations),(SELECT count(*) FROM river_job WHERE kind='service_lifecycle')`).Scan(&inspections, &operations, &jobs); err != nil || inspections != 2 || operations != 1 || jobs != 1 {
		t.Fatal("receipt reads lacked audits or created work", err)
	}
}

func TestLifecycleReceiptScopeAndBrowserBoundaries(t *testing.T) {
	s, actions := fixture(t)
	ctx := context.Background()
	key := "66666666-6666-4666-8666-666666666666"
	if _, err := actions.Submit(ctx, "oidc-owner:"+alice, instance, lifecycle.Request{Key: key, Kind: "service", State: "running", Reason: "maintenance"}); err != nil {
		t.Fatal(err)
	}
	s.Settings.Identities = append(s.Settings.Identities, Identity{"http://localhost:5556", "bob", "operate"})
	path := "/api/operator/deployments/" + instance + "/lifecycle-requests/" + key
	for _, tc := range []struct {
		owner, path, bearer string
		status              int
	}{
		{bob, path, "", 404}, {otherIssuer, path, "", 404}, {alice, path + "?resolver=private", "", 400},
		{alice, strings.Replace(path, key, "77777777-7777-4777-8777-777777777777", 1), "", 404},
		{alice, strings.Replace(path, instance, "88888888-8888-4888-8888-888888888888", 1), "", 404},
		{alice, strings.Replace(path, key, "invalid", 1), "", 404}, {alice, path, "Bearer private", 403},
	} {
		mux := http.NewServeMux()
		s.Register(sessionFixture{owner: tc.owner}, actions)(mux)
		request := httptest.NewRequest("GET", tc.path, nil)
		if tc.bearer != "" {
			request.Header.Set("Authorization", tc.bearer)
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != tc.status || strings.Contains(response.Body.String(), key) || strings.Contains(response.Body.String(), "PRIVATE") {
			t.Fatal("receipt boundary leaked or accepted scope", response.Code)
		}
	}
	var audits int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM support_inspections`).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("denied receipt inspection audited a false success", err)
	}
	s.Settings = Settings{}
	mux := http.NewServeMux()
	s.Register(sessionFixture{owner: alice}, actions)(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
	if response.Code != 404 {
		t.Fatal("default receipt authority open")
	}
}

func TestLifecycleReceiptRecoveryAndAuditFailureReturnNoData(t *testing.T) {
	for _, mode := range []string{"held", "missing-ledger", "audit-failure", "revoked", "inspect"} {
		t.Run(mode, func(t *testing.T) {
			s, actions := fixture(t)
			ctx := context.Background()
			key := "66666666-6666-4666-8666-666666666666"
			op, err := actions.Submit(ctx, "oidc-owner:"+alice, instance, lifecycle.Request{Key: key, Kind: "service", State: "running", Reason: "maintenance"})
			if err != nil {
				t.Fatal(err)
			}
			before := operatorReceiptWork(t, s)
			role := "operate"
			switch mode {
			case "held":
				_, err = s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32))
			case "missing-ledger":
				_, err = s.Pool.Exec(ctx, `DROP TABLE management_recovery`)
			case "audit-failure":
				_, err = s.Pool.Exec(ctx, `CREATE FUNCTION reject_receipt_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'PRIVATE_AUDIT_ERROR'; END $$; CREATE TRIGGER receipt_audit BEFORE INSERT ON support_inspections FOR EACH ROW EXECUTE FUNCTION reject_receipt_audit()`)
			case "revoked":
				s.Settings = Settings{}
			case "inspect":
				s.Settings.Identities[0].Role = "inspect"
				role = "inspect"
			}
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := s.LifecycleRequest(ctx, alice, role, instance, key)
			if mode == "inspect" {
				if err != nil || receipt.ID != op.ID {
					t.Fatal("inspection role lost its original receipt", err)
				}
			} else if err == nil || receipt.ID != "" {
				t.Fatal("unconfirmed receipt disclosed data")
			}
			if mode == "held" && !errors.Is(err, recovery.ErrHeld) {
				t.Fatal(err)
			}
			var audits int
			if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM support_inspections`).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			want := 0
			if mode == "inspect" {
				want = 1
			}
			if audits != want || operatorReceiptWork(t, s) != before {
				t.Fatal("receipt lookup changed work or committed false inspection")
			}
		})
	}
}

func TestLifecycleReceiptHoldDuringAuditWaitRollsBackInspection(t *testing.T) {
	s, actions := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	key := "66666666-6666-4666-8666-666666666666"
	if _, err := actions.Submit(ctx, "oidc-owner:"+alice, instance, lifecycle.Request{Key: key, Kind: "service", State: "running", Reason: "maintenance"}); err != nil {
		t.Fatal(err)
	}
	before := operatorReceiptWork(t, s)
	block, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback(context.Background())
	if _, err := block.Exec(ctx, `LOCK TABLE support_inspections IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		receipt LifecycleReceipt
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		receipt, err := s.LifecycleRequest(ctx, alice, "operate", instance, key)
		done <- outcome{receipt, err}
	}()
	for {
		var waiting bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'INSERT INTO support_inspections%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("receipt did not reach audit lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := block.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if !errors.Is(result.err, recovery.ErrHeld) || result.receipt.ID != "" {
			t.Fatal("held audit wait returned receipt")
		}
	case <-ctx.Done():
		t.Fatal("receipt remained blocked")
	}
	var audits int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM support_inspections`).Scan(&audits); err != nil || audits != 0 || operatorReceiptWork(t, s) != before {
		t.Fatal("held receipt persisted inspection or changed work", err)
	}
}

func operatorReceiptWork(t *testing.T, s *Store) string {
	t.Helper()
	var value string
	if err := s.Pool.QueryRow(context.Background(), `SELECT json_build_object('operations',(SELECT coalesce(json_agg(o ORDER BY id),'[]') FROM service_operations o),'events',(SELECT coalesce(json_agg(e ORDER BY id),'[]') FROM service_operation_events e),'jobs',(SELECT coalesce(json_agg(j ORDER BY id),'[]') FROM river_job j))::text`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
