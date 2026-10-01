package deployment

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestAdmissionPreservesReplayAndRejectsNewIntent(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	var owner, org string
	if err = pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','alice','Alice') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Studio',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	request := Request{Kind: "test", Name: "original", Region: "local", Key: "stable"}
	first, err := store.Submit(ctx, owner, org, request)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := store.SetAdmission(ctx, "operator", "local_qualification", false)
	if err != nil || closed.Open {
		t.Fatalf("close: %+v %v", closed, err)
	}
	replay, err := store.Submit(ctx, owner, org, request)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("retained retry: %+v %v", replay, err)
	}
	if _, err = store.Get(ctx, owner, first.ID); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"test", "agent"} {
		t.Run(kind, func(t *testing.T) {
			_, err := store.Submit(ctx, owner, org, Request{Kind: kind, Name: "new-" + kind, Region: "local", Key: "new-" + kind})
			if !errors.Is(err, ErrAdmissionClosed) {
				t.Fatalf("new request: %v", err)
			}
		})
	}
	request.Name = "changed"
	if _, err = store.Submit(ctx, owner, org, request); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retained payload: %v", err)
	}
	if _, err = store.Submit(ctx, "00000000-0000-0000-0000-000000000000", org, request); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign owner: %v", err)
	}
	if _, err = store.SetAdmission(ctx, "operator", "maintenance", false); err != nil {
		t.Fatal(err)
	}
	var deployments, jobs, events int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM deployments),(SELECT count(*) FROM river_job),(SELECT count(*) FROM deployment_admission_events)`).Scan(&deployments, &jobs, &events); err != nil || deployments != 1 || jobs != 1 || events != 1 {
		t.Fatalf("retained counts %d %d %d: %v", deployments, jobs, events, err)
	}
	reopened, err := store.SetAdmission(ctx, "operator", "local_qualification", true)
	if err != nil || !reopened.Open {
		t.Fatalf("reopen: %v", err)
	}
	if _, err = store.Submit(ctx, owner, org, Request{Name: "after-open", Region: "local", Key: "after-open"}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	operationReply(response, Operation{}, ErrAdmissionClosed)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "same request key") {
		t.Fatalf("pause response: %d %s", response.Code, response.Body.String())
	}
}

func TestAdmissionLockAuditAndRecovery(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = checkAdmission(ctx, tx); err != nil {
		t.Fatal(err)
	}
	timeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = store.SetAdmission(timeout, "operator", "maintenance", false)
	cancel()
	if err == nil {
		t.Fatal("close bypassed the admitted transaction lock")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { _, err := store.SetAdmission(ctx, "operator", "maintenance", false); results <- err }()
	}
	for i := 0; i < 8; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM deployment_admission_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent effective audit: %d %v", count, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE deployment_admission_events ADD CONSTRAINT reject_open CHECK(requested_open=false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetAdmission(ctx, "operator", "maintenance", true); err == nil {
		t.Fatal("failed audit accepted")
	}
	state, err := store.Admission(ctx)
	if err != nil || state.Open {
		t.Fatalf("audit rollback: %+v %v", state, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE deployment_admission_events DROP CONSTRAINT reject_open; INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetAdmission(ctx, "operator", "maintenance", true); !errors.Is(err, recovery.ErrHeld) {
		t.Fatalf("recovery hold: %v", err)
	}
	heldTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = checkAdmission(ctx, heldTx); !errors.Is(err, recovery.ErrHeld) {
		t.Fatalf("held admission: %v", err)
	}
	if err = heldTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetAdmission(ctx, "operator", "unsupported", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reason: %v", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM deployment_admission; DELETE FROM management_recovery`); err != nil {
		t.Fatal(err)
	}
	missing, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Rollback(ctx)
	if err = checkAdmission(ctx, missing); err == nil {
		t.Fatal("missing policy admitted request")
	}
}
