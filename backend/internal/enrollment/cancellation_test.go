package enrollment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

func TestNativeFailureRetainsDispatchCheckpoint(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentFixture{failure: true}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if err = worker.Work(ctx, job(op.ID, 1)); err == nil {
		t.Fatal("native failure reported success")
	}
	var started bool
	if err = f.pool.QueryRow(ctx, `SELECT dispatch_started_at IS NOT NULL FROM enrollment_operations WHERE id=$1`, op.ID).Scan(&started); err != nil || !started || agent.calls != 1 {
		t.Fatalf("dispatch checkpoint must survive a lost native result: %v %v %d", started, err, agent.calls)
	}
}

func TestCancellationIsStableScopedAndFencesRetainedJobs(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	request := Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
	op, err := f.store.Submit(ctx, f.owner, f.deployment, request)
	if err != nil || !op.CanCancel {
		t.Fatal("fresh queued request is not cancellable", err)
	}
	if _, err = f.store.Cancel(ctx, f.bob, op.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign cancellation accepted")
	}
	var wait sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := f.store.Cancel(ctx, f.owner, op.ID)
			if err == nil && (got.State != "cancelled" || got.CanCancel || got.Code != "" || got.WithdrawalRequested) {
				err = errors.New("inconsistent cancellation")
			}
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var events, jobs int
	if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM enrollment_events WHERE enrollment_id=$1),(SELECT count(*) FROM river_job)`, op.ID).Scan(&events, &jobs); err != nil || events != 2 || jobs != 1 {
		t.Fatal("duplicate cancellation or removed job", events, jobs, err)
	}
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	for i := 0; i < 3; i++ {
		if err = worker.Work(ctx, job(op.ID, 3)); err != nil {
			t.Fatal(err)
		}
	}
	if err = worker.recordFailure(ctx, op.ID, 0); err != nil || agent.calls != 0 {
		t.Fatal("cancelled job executed native work", err)
	}
	got, err := f.store.Submit(ctx, f.owner, f.deployment, request)
	if err != nil || got.State != "cancelled" {
		t.Fatal("exact retry replaced cancelled request", err)
	}
	if _, err = f.store.Refresh(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled request refreshed")
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled request reached native withdrawal")
	}
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Recover: true}); err != nil {
		t.Fatal("bounded recovery refused", err)
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil || agent.calls != 0 {
		t.Fatal("superseded cancellation executed native work", err)
	}
}

type blockingEnrollment struct{ entered, release chan struct{} }

func (a *blockingEnrollment) Enroll(ctx context.Context, _ nodeagent.Peer, id, ticket string) (nodeagent.Enrollment, error) {
	close(a.entered)
	select {
	case <-ctx.Done():
		return nodeagent.Enrollment{}, ctx.Err()
	case <-a.release:
	}
	return nodeagent.Enrollment{}, errors.New("native response lost")
}
func (a *blockingEnrollment) WithdrawEnrollment(context.Context, nodeagent.Peer, string, string) (nodeagent.Enrollment, error) {
	return nodeagent.Enrollment{}, errors.New("unexpected withdrawal")
}

func TestCancellationCannotRaceDispatchedOrLostNativeResponse(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &blockingEnrollment{make(chan struct{}), make(chan struct{})}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	workDone := make(chan error, 1)
	go func() { workDone <- worker.Work(ctx, job(op.ID, 1)) }()
	select {
	case <-agent.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("native dispatch did not start")
	}
	// Read without the deployment lock while the RPC holds it. The durable
	// checkpoint must already be visible, even before native result commit.
	var started bool
	if err = f.pool.QueryRow(ctx, `SELECT dispatch_started_at IS NOT NULL FROM enrollment_operations WHERE id=$1`, op.ID).Scan(&started); err != nil || !started {
		close(agent.release)
		t.Fatal("native RPC preceded committed checkpoint", err)
	}
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = f.store.Cancel(short, f.owner, op.ID)
	cancel()
	if err == nil {
		close(agent.release)
		t.Fatal("in-flight request cancelled")
	}
	close(agent.release)
	if err = <-workDone; err == nil {
		t.Fatal("lost response confirmed")
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || got.State != "pending" || got.CanCancel {
		t.Fatal("lost response appeared cancellable", err)
	}
	if _, err = f.store.Cancel(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("dispatched request cancelled after lost response")
	}
}

func TestCancellationFailsClosedAndAuditRollsBack(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Cancel(ctx, "invalid", op.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("invalid reference accepted")
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION reject_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER reject_cancel_audit BEFORE INSERT ON enrollment_events FOR EACH ROW EXECUTE FUNCTION reject_cancel_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Cancel(ctx, f.owner, op.ID); err == nil {
		t.Fatal("audit failure accepted")
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || got.State != "pending" || !got.CanCancel || len(got.Events) != 1 {
		t.Fatal("audit failure left cancellation", err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Cancel(ctx, f.owner, op.ID); err == nil {
		t.Fatal("held cancellation accepted")
	}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: &agentFixture{}}
	if _, err = worker.reserveDispatch(ctx, op.ID); err == nil {
		t.Fatal("held native reservation accepted")
	}
	if _, err = f.pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Cancel(ctx, f.owner, op.ID); err == nil {
		t.Fatal("missing recovery ledger accepted")
	}
}

func TestCancellationUpgradeConservativelyFencesRetainedPending(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err = f.pool.QueryRow(ctx, `SELECT (to_jsonb(e)-'dispatch_started_at')::text FROM enrollment_operations e WHERE id=$1`, op.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// Reconstruct only the new DDL in this disposable database.
	if _, err = f.pool.Exec(ctx, `DROP TABLE billing_checkout_events,billing_checkout_requests; DROP TABLE endpoint_claim_events,endpoint_claims; ALTER TABLE endpoint_choices DROP CONSTRAINT endpoint_choice_claim_identity; ALTER TABLE enrollment_operations DROP CONSTRAINT enrollment_cancellation_checkpoint; ALTER TABLE enrollment_operations DROP COLUMN dispatch_started_at; UPDATE schema_version SET version=28`); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Apply(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	var after string
	var started bool
	if err = f.pool.QueryRow(ctx, `SELECT (to_jsonb(e)-'dispatch_started_at')::text,dispatch_started_at=updated_at FROM enrollment_operations e WHERE id=$1`, op.ID).Scan(&after, &started); err != nil || before != after || !started {
		t.Fatal("upgrade changed retained fields or failed to fence", err)
	}
	if _, err = f.store.Cancel(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy ambiguous pending cancelled")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE enrollment_operations SET state='cancelled' WHERE id=$1`, op.ID); err == nil {
		t.Fatal("database accepted dispatched cancellation")
	}
}

func TestCancellationRequiresOriginalAndCurrentOwner(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=(SELECT organization_id FROM deployments WHERE id=$2)`, f.bob, f.deployment); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{f.owner, f.bob} {
		if _, err = f.store.Cancel(ctx, owner, op.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("changed owner cancelled original request")
		}
	}
}

func TestDispatchReservationFailureNeverReachesNative(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION reject_dispatch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'checkpoint unavailable'; END $$; CREATE TRIGGER reject_dispatch BEFORE UPDATE ON enrollment_operations FOR EACH ROW EXECUTE FUNCTION reject_dispatch()`); err != nil {
		t.Fatal(err)
	}
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if err = worker.Work(ctx, job(op.ID, 3)); err == nil || agent.calls != 0 {
		t.Fatal("native command preceded failed dispatch checkpoint")
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || !got.CanCancel || got.State != "pending" || len(got.Events) != 1 {
		t.Fatal("failed checkpoint changed intent", err)
	}
}
