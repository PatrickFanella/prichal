package endpointplan

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/orgaccess"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

func selection(key string) ChoiceRequest {
	return ChoiceRequest{"PDS.creator.test", []string{"app.other.test"}, key, true}
}
func TestChoiceStableIntentAndCancellation(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	var first Choice
	for i := 0; i < 3; i++ {
		c, err := s.SaveChoice(ctx, alice, org, selection("save-one"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = c
		} else if c.ID != first.ID || c.ProofValue != first.ProofValue {
			t.Fatal("retry changed proof")
		}
	}
	if first.Hostname != "pds.creator.test" || first.State != "awaiting_ownership" || first.CanActivate || first.ProofName != "_pdsaas.pds.creator.test" || len(first.ProofValue) != 60 || first.ExpiresAt.Sub(first.CreatedAt) < 7*24*time.Hour {
		t.Fatal("unsafe instruction")
	}
	request := selection("save-one")
	request.ApplicationHosts = []string{"different.test"}
	if _, err := s.SaveChoice(ctx, alice, org, request); !errors.Is(err, ErrConflict) {
		t.Fatal("changed payload")
	}
	if _, err := s.SaveChoice(ctx, alice, org, selection("save-two")); !errors.Is(err, ErrConflict) {
		t.Fatal("second pending choice")
	}
	if _, err := s.GetChoice(ctx, bob, org, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign proof")
	}
	if _, err := s.CancelChoice(ctx, bob, org, first.ID, "cancel", true); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign cancellation")
	}
	if _, err := s.CancelChoice(ctx, alice, org, first.ID, "cancel", false); !errors.Is(err, ErrInvalid) {
		t.Fatal("unreviewed cancellation")
	}
	c, err := s.CancelChoice(ctx, alice, org, first.ID, "cancel", true)
	if err != nil || c.State != "cancelled" {
		t.Fatal(err)
	}
	if _, err = s.CancelChoice(ctx, alice, org, first.ID, "cancel", true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelChoice(ctx, alice, org, first.ID, "changed", true); !errors.Is(err, ErrConflict) {
		t.Fatal("changed cancellation key")
	}
	prior, err := s.SaveChoice(ctx, alice, org, selection("save-one"))
	if err != nil || prior.State != "cancelled" || prior.ProofValue != first.ProofValue {
		t.Fatal("cancelled retry resurrected")
	}
	current, err := s.CurrentChoice(ctx, alice, org)
	if err != nil || current != nil {
		t.Fatal("cancelled remains pending")
	}
	second, err := s.SaveChoice(ctx, alice, org, selection("save-two"))
	if err != nil || second.ProofValue == first.ProofValue {
		t.Fatal("replacement proof reuse")
	}
	var events int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM endpoint_choice_events`).Scan(&events); err != nil || events != 3 {
		t.Fatal("duplicate/missing audit")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetChoice(ctx, alice, org, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("former owner read")
	}
	if _, err = s.GetChoice(ctx, bob, org, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("handover granted original proof")
	}
}
func TestChoiceConcurrentAuditAndNoGlobalReservation(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.SaveChoice(ctx, alice, org, selection("concurrent"))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var events int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM endpoint_choice_events`).Scan(&events); err != nil || events != 1 {
		t.Fatal("duplicate audit")
	}
	other := "44444444-4444-4444-8444-444444444444"
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES($1,$2,'other owner')`, other, bob); err != nil {
		t.Fatal(err)
	}
	a, err := s.CurrentChoice(ctx, alice, org)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveChoice(ctx, bob, other, selection("same-domain"))
	if err != nil || a.ProofValue == b.ProofValue {
		t.Fatal("unverified domain globally reserved")
	}
}
func TestChoiceAuditRollbackHoldAndNativeGuard(t *testing.T) {
	for _, mode := range []string{"audit", "held", "native", "unconfirmed", "production"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			ctx := context.Background()
			r := selection("request")
			switch mode {
			case "audit":
				_, err := s.Pool.Exec(ctx, `ALTER TABLE endpoint_choice_events ADD CONSTRAINT reject_save CHECK(action<>'save')`)
				if err != nil {
					t.Fatal(err)
				}
			case "held":
				_, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,decode(repeat('00',32),'hex'),'held')`)
				if err != nil {
					t.Fatal(err)
				}
			case "native":
				_, err := s.Pool.Exec(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,managed_hostname) VALUES($1,'retained','local','retained','agent','retained.localhost')`, org)
				if err != nil {
					t.Fatal(err)
				}
			case "unconfirmed":
				r.Confirmed = false
			case "production":
				s.Local = false
			}
			if _, err := s.SaveChoice(ctx, alice, org, r); err == nil {
				t.Fatal("selection permitted")
			}
			var choices, events int
			if err := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM endpoint_choices),(SELECT count(*) FROM endpoint_choice_events)`).Scan(&choices, &events); err != nil || choices != 0 || events != 0 {
				t.Fatal("partial selection")
			}
		})
	}
	s := fixture(t)
	ctx := context.Background()
	c, err := s.SaveChoice(ctx, alice, org, selection("saved"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `ALTER TABLE endpoint_choice_events ADD CONSTRAINT reject_cancel CHECK(action<>'cancel') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); err == nil {
		t.Fatal("unaudited cancellation")
	}
	current, err := s.CurrentChoice(ctx, alice, org)
	if err != nil || current == nil || current.State != "awaiting_ownership" {
		t.Fatal("partial cancellation")
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,decode(repeat('00',32),'hex'),'held')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("held cancellation")
	}
}
func TestChoiceBlocksNewNativeAndHandoverWithoutJobs(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	if err := deployment.Migrate(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	deployments, err := deployment.New(s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.SaveChoice(ctx, alice, org, selection("saved"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE endpoint_choices SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = deployments.Submit(ctx, alice, org, deployment.Request{Kind: "agent", Name: "native", Region: "local", Key: "native"}); !errors.Is(err, deployment.ErrEndpointPending) {
		t.Fatalf("silent fallback: %v", err)
	}
	op, err := deployments.Submit(ctx, alice, org, deployment.Request{Kind: "test", Name: "simulation", Region: "local", Key: "simulation"})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := deployments.Submit(ctx, alice, org, deployment.Request{Kind: "test", Name: "simulation", Region: "local", Key: "simulation"})
	if err != nil || retry.ID != op.ID {
		t.Fatal("retained simulation unavailable")
	}
	var jobs int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM river_job`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatal("native work queued")
	}
	// Offer/accept both check pending endpoint custody independently of deployments.
	if _, err = s.Pool.Exec(ctx, `DELETE FROM river_job; DELETE FROM deployment_events; DELETE FROM deployments`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) VALUES($1,$2,'administrator','active')`, org, bob); err != nil {
		t.Fatal(err)
	}
	access := orgaccess.Store{Pool: s.Pool, Enabled: true}
	if _, err = access.Transfer(ctx, alice, org, bob, "55555555-5555-4555-8555-555555555555", true); !errors.Is(err, orgaccess.ErrConflict) {
		t.Fatal("pending choice handed over")
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO organization_transfers(id,organization_id,previous_owner_id,next_owner_id,idempotency_key) VALUES('66666666-6666-4666-8666-666666666666',$1,$2,$3,'77777777-7777-4777-8777-777777777777')`, org, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err = access.AcceptTransfer(ctx, bob, "66666666-6666-4666-8666-666666666666", true); !errors.Is(err, orgaccess.ErrConflict) {
		t.Fatal("accept transferred pending proof")
	}
}
func TestChoiceHTTPReviewAndScope(t *testing.T) {
	s := fixture(t)
	a := &sessions{owner: alice}
	mux := http.NewServeMux()
	s.RegisterChoices(a)(mux)
	path := "/api/organizations/" + org + "/endpoint-choices"
	for _, tc := range []struct {
		body, bearer string
		status       int
	}{
		{`{"hostname":"creator.test","applicationHosts":[],"idempotencyKey":"one","confirmed":true}`, "", 200},
		{`{"hostname":"creator.test","applicationHosts":[],"idempotencyKey":"one","confirmed":false}`, "", 400},
		{`{"hostname":"creator.test","applicationHosts":[],"idempotencyKey":"one","confirmed":true,"private":"SECRET"}`, "", 400},
		{`{}`, "Bearer private", 403},
		{strings.Repeat("x", 4097), "", 400},
	} {
		r := httptest.NewRequest("POST", path, strings.NewReader(tc.body))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "fixture-csrf")
		if tc.bearer != "" {
			r.Header.Set("Authorization", tc.bearer)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal("choice HTTP boundary")
		}
	}
	if !a.calledMutation {
		t.Fatal("review omitted CSRF")
	}
	s.Local = false
	r := httptest.NewRequest("GET", "/api/organizations/"+org+"/endpoint-choice", nil)
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "fixture-csrf")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("production selection exposed")
	}
}

func TestChoiceSerializesWithNativeSubmission(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	if err := deployment.Migrate(ctx, s.Pool); err != nil {
		t.Fatal(err)
	}
	d, err := deployment.New(s.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO pilot_entitlement_operations(organization_id,idempotency_key,generation,state,plan,duration_hours,reason,actor,expires_at) VALUES($1,'77777777-7777-4777-8777-777777777777',1,'active','local-single',24,'qualification','fixture',clock_timestamp()+interval '24 hours')`, org); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; _, e := s.SaveChoice(ctx, alice, org, selection("race")); results <- e }()
	go func() {
		<-start
		_, e := d.Submit(ctx, alice, org, deployment.Request{Kind: "agent", Name: "native", Region: "local", Key: "native"})
		results <- e
	}()
	close(start)
	select {
	case <-results:
		t.Fatal("operation bypassed organization lock")
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a, b := <-results, <-results
	if (a == nil) == (b == nil) {
		t.Fatalf("expected exactly one intent: %v %v", a, b)
	}
	blocked := a
	if blocked == nil {
		blocked = b
	}
	if !errors.Is(blocked, ErrConflict) && !errors.Is(blocked, deployment.ErrEndpointPending) {
		t.Fatal(blocked)
	}
	var choices, native, jobs int
	if err = s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM endpoint_choices),(SELECT count(*) FROM deployments),(SELECT count(*) FROM river_job)`).Scan(&choices, &native, &jobs); err != nil || choices+native != 1 || jobs != native {
		t.Fatal("partial/conflicting intent")
	}
	if native == 1 {
		op, err := d.Submit(ctx, alice, org, deployment.Request{Kind: "agent", Name: "native", Region: "local", Key: "native"})
		if err != nil || op.ID == "" {
			t.Fatal("native retry lost")
		}
	}
}

func TestChoiceBoundsAndPendingHandover(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	tooLong := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 49) + ".test"
	r := selection("too-long")
	r.Hostname = tooLong
	if _, err := s.SaveChoice(ctx, alice, org, r); !errors.Is(err, ErrInvalid) {
		t.Fatal("TXT name exceeds bound")
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO organization_transfers(organization_id,previous_owner_id,next_owner_id,idempotency_key) VALUES($1,$2,$3,'77777777-7777-4777-8777-777777777777')`, org, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveChoice(ctx, alice, org, selection("handover")); !errors.Is(err, ErrConflict) {
		t.Fatal("pending handover gained proof custody")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE organization_transfers SET state='expired'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		key := strings.Repeat("x", i+1)
		c, err := s.SaveChoice(ctx, alice, org, selection(key))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CancelChoice(ctx, alice, org, c.ID, "cancel", true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveChoice(ctx, alice, org, selection("thirty-one")); !errors.Is(err, ErrConflict) {
		t.Fatal("retained request cap")
	}
}
