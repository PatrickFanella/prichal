package enrollment

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

func TestAdmissionPreservesRetainedEnrollmentAndStopsNewRequests(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	request := Request{Key: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}
	first, err := f.store.Submit(ctx, f.owner, f.deployment, request)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := f.store.SetAdmission(ctx, "operator", "local_qualification", false)
	if err != nil || closed.Open {
		t.Fatal("close failed")
	}
	replay, err := f.store.Submit(ctx, f.owner, f.deployment, request)
	if err != nil || replay.ID != first.ID {
		t.Fatal("retained enrollment refused")
	}
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}); !errors.Is(err, ErrAdmissionClosed) {
		t.Fatal("new request admitted")
	}
	request.Recover = true
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, request); !errors.Is(err, ErrConflict) {
		t.Fatal("changed retry admitted")
	}
	if _, err = f.store.Submit(ctx, f.bob, f.deployment, request); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign enrollment")
	}
	if _, err = f.store.Get(ctx, f.owner, first.ID); err != nil {
		t.Fatal(err)
	}
	// Already-admitted work can prepare its invitation while admission is closed.
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if err = worker.Work(ctx, job(first.ID, 1)); err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.Get(ctx, f.owner, first.ID)
	if err != nil || ready.State != "invitation_ready" || agent.calls != 1 {
		t.Fatal("admitted work interrupted")
	}
	if _, err = f.store.SetAdmission(ctx, "operator", "maintenance", false); err != nil {
		t.Fatal(err)
	}
	var operations, jobs, audits int
	if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM enrollment_operations),(SELECT count(*) FROM river_job),(SELECT count(*) FROM enrollment_admission_events)`).Scan(&operations, &jobs, &audits); err != nil || operations != 1 || jobs != 1 || audits != 1 {
		t.Fatal("pause mutated retained work")
	}
	if _, err = f.store.SetAdmission(ctx, "operator", "local_qualification", true); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", Recover: true}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	operationReply(response, Operation{}, ErrAdmissionClosed)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "Already-issued invitations") {
		t.Fatal("pause guidance")
	}
}
func TestAdmissionLockAuditAndRecovery(t *testing.T) {
	f := newStoreFixture(t)
	pool := f.pool
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := f.store
	var err error
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
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM enrollment_admission_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("concurrent effective audit: %d %v", count, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE enrollment_admission_events ADD CONSTRAINT reject_open CHECK(requested_open=false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetAdmission(ctx, "operator", "maintenance", true); err == nil {
		t.Fatal("failed audit accepted")
	}
	state, err := store.Admission(ctx)
	if err != nil || state.Open {
		t.Fatalf("audit rollback: %+v %v", state, err)
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE enrollment_admission_events DROP CONSTRAINT reject_open; INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
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
	if _, err = pool.Exec(ctx, `DELETE FROM enrollment_admission; DELETE FROM management_recovery`); err != nil {
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
