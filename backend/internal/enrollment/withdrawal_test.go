package enrollment

import (
	"context"
	"errors"
	"sync"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
)

func (a *agentFixture) WithdrawEnrollment(ctx context.Context, peer nodeagent.Peer, id, ticket string) (nodeagent.Enrollment, error) {
	if a.verified || a.failure {
		return a.Enroll(ctx, peer, id, ticket)
	}
	a.calls++
	return nodeagent.Enrollment{InstanceID: id, Ticket: ticket, State: "withdrawn"}, nil
}

func TestWithdrawalIsAtomicScopedAndDispatchesInsteadOfIssuing(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("unconfirmed pending issuance accepted withdrawal")
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	agent.calls = 0
	if _, err = f.store.Withdraw(ctx, f.bob, op.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign withdrawal accepted")
	}
	var wait sync.WaitGroup
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, err := f.store.Withdraw(ctx, f.owner, op.ID)
			if err != nil {
				failures <- err
				return
			}
			if got.State != "withdrawing" || got.Code != "" {
				failures <- errors.New("withdrawal not retained")
			}
		}()
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var audits, jobs int
	if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM enrollment_events WHERE enrollment_id=$1 AND state='withdrawing'),(SELECT count(*) FROM river_job)`, op.ID).Scan(&audits, &jobs); err != nil || audits != 1 || jobs != 2 {
		t.Fatalf("non-atomic duplicate withdrawal: %d %d %v", audits, jobs, err)
	}
	if _, err = f.store.Refresh(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("withdrawing invitation refreshed")
	}
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Recover: true}); !errors.Is(err, ErrConflict) {
		t.Fatal("withdrawal bypassed by replacement")
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || got.State != "withdrawn" || got.Code != "" {
		t.Fatal("withdrawal not confirmed")
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil || agent.calls != 1 {
		t.Fatal("duplicate job touched confirmed withdrawal")
	}
	got, err = f.store.Withdraw(ctx, f.owner, op.ID)
	if err != nil || got.State != "withdrawn" {
		t.Fatal("stable withdrawal retry failed")
	}
	if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Recover: true}); err != nil {
		t.Fatal("confirmed withdrawal did not permit bounded new recovery")
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("superseded withdrawal accepted")
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil || agent.calls != 1 {
		t.Fatal("superseded withdrawal repeated host work")
	}
}

func TestWithdrawalClearsInvitationAndPreservesCompletedAccount(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Withdraw(ctx, f.owner, op.ID)
	if err != nil || got.State != "withdrawing" || got.Code != "" {
		t.Fatal("issued code remained disclosed")
	}
	agent.verified = true
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	got, err = f.store.Withdraw(ctx, f.owner, op.ID)
	if err != nil || got.State != "verified" || got.Code != "" {
		t.Fatal("completed account lost")
	}
	before := agent.calls
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil || agent.calls != before {
		t.Fatal("completed withdrawal restarted native action")
	}
}

func TestWithdrawalRefusesHeldRecoveryAndRollsBackUnavailableQueue(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: &agentFixture{}}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION reject_withdrawal_job() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture queue unavailable'; END $$; CREATE TRIGGER reject_withdrawal_job BEFORE INSERT ON river_job FOR EACH ROW EXECUTE FUNCTION reject_withdrawal_job()`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); err == nil {
		t.Fatal("queue failure accepted")
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || got.State != "invitation_ready" || len(got.Events) != 2 {
		t.Fatal("queue failure left intent/audit")
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); err == nil {
		t.Fatal("held withdrawal accepted")
	}
	if _, err = f.store.Get(ctx, f.owner, op.ID); err == nil {
		t.Fatal("held enrollment disclosed invitation")
	}
	if err = worker.recordFailure(ctx, op.ID, 0); err == nil {
		t.Fatal("held failure recording changed management state")
	}
}

func TestOlderWorkerFailureCannotFailNewWithdrawalIntent(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	agent.failure = true
	attempt := -1
	if err = worker.perform(ctx, op.ID, &attempt); err == nil || attempt != 0 {
		t.Fatal("old issuance failure fixture")
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); err != nil {
		t.Fatal(err)
	}
	if err = worker.recordFailure(ctx, op.ID, attempt); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || got.State != "withdrawing" || len(got.Events) != 3 {
		t.Fatal("old failure clobbered new owner intent")
	}
}

func TestUnconfirmedWithdrawalCannotIssueOrRetryWithoutBound(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentFixture{}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	agent.failure = true
	for attempt := 0; attempt < 3; attempt++ {
		if _, err = f.store.Withdraw(ctx, f.owner, op.ID); err != nil {
			t.Fatal(err)
		}
		if err = worker.Work(ctx, job(op.ID, 3)); err == nil {
			t.Fatal("native failure reported confirmation")
		}
		got, err := f.store.Get(ctx, f.owner, op.ID)
		if err != nil || got.State != "failed" || got.Code != "" {
			t.Fatal("unconfirmed withdrawal disclosed invitation")
		}
		before := agent.calls
		if err = worker.Work(ctx, job(op.ID, 1)); err != nil || agent.calls != before {
			t.Fatal("discarded withdrawal repeated native action")
		}
		if _, err = f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Recover: true}); !errors.Is(err, ErrConflict) {
			t.Fatal("unconfirmed withdrawal replaced")
		}
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("unbounded withdrawal attempts")
	}
}

func TestWithdrawalRequiresOriginalAndCurrentOwner(t *testing.T) {
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
		if _, err = f.store.Withdraw(ctx, owner, op.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("changed owner withdrew original invitation")
		}
		if _, err = f.store.Get(ctx, owner, op.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("changed owner read original invitation")
		}
	}
}

func TestWithdrawalMigrationPreservesPreviousSchemaIntent(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err = f.pool.QueryRow(ctx, `SELECT (to_jsonb(e)-'withdrawal_requested_at'-'withdrawal_attempts'-'dispatch_started_at')::text FROM enrollment_operations e WHERE id=$1`, op.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// Reconstruct schema 27 only in this disposable integration database.
	if _, err = f.pool.Exec(ctx, `DROP TABLE billing_checkout_notifications; DROP TABLE billing_checkout_events,billing_checkout_requests; DROP TABLE endpoint_claim_events,endpoint_claims; ALTER TABLE endpoint_choices DROP CONSTRAINT endpoint_choice_claim_identity; ALTER TABLE enrollment_operations DROP CONSTRAINT enrollment_cancellation_checkpoint; ALTER TABLE enrollment_operations DROP COLUMN dispatch_started_at; ALTER TABLE enrollment_operations DROP CONSTRAINT enrollment_withdrawal_intent; ALTER TABLE enrollment_operations DROP COLUMN withdrawal_requested_at, DROP COLUMN withdrawal_attempts; ALTER TABLE enrollment_operations DROP CONSTRAINT enrollment_operations_state_check; ALTER TABLE enrollment_operations ADD CONSTRAINT enrollment_operations_state_check CHECK(state IN ('pending','invitation_ready','account_created','verified','expired','failed','superseded')); UPDATE schema_version SET version=27`); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Apply(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	var after string
	var attempts int
	var requested bool
	if err = f.pool.QueryRow(ctx, `SELECT (to_jsonb(e)-'withdrawal_requested_at'-'withdrawal_attempts'-'dispatch_started_at')::text,withdrawal_attempts,withdrawal_requested_at IS NOT NULL FROM enrollment_operations e WHERE id=$1`, op.ID).Scan(&after, &attempts, &requested); err != nil || after != before || attempts != 0 || requested {
		t.Fatal("upgrade changed previous intent")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE enrollment_operations SET state='withdrawing' WHERE id=$1`, op.ID); err == nil {
		t.Fatal("database accepted withdrawal without intent")
	}
}

func TestWithdrawalAuditAndMissingRecoveryLedgerFailClosed(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	if _, err := f.store.Withdraw(ctx, "invalid", "invalid"); !errors.Is(err, ErrNotFound) {
		t.Fatal("invalid references accepted")
	}
	op, err := f.store.Submit(ctx, f.owner, f.deployment, Request{Key: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: &agentFixture{}}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION reject_withdrawal_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture audit unavailable'; END $$; CREATE TRIGGER reject_withdrawal_audit BEFORE INSERT ON enrollment_events FOR EACH ROW EXECUTE FUNCTION reject_withdrawal_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); err == nil {
		t.Fatal("unavailable audit accepted intent")
	}
	if err = worker.recordFailure(ctx, op.ID, 0); err == nil {
		t.Fatal("unavailable audit accepted worker failure checkpoint")
	}
	got, err := f.store.Get(ctx, f.owner, op.ID)
	if err != nil || got.State != "invitation_ready" || got.WithdrawalAttempts != 0 || len(got.Events) != 2 {
		t.Fatal("audit failure did not roll back intent")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE deployments SET state='failed' WHERE id=$1`, f.deployment); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Withdraw(ctx, f.owner, op.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("failed deployment accepted withdrawal")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE deployments SET state='ready' WHERE id=$1;`, f.deployment); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Get(ctx, f.owner, op.ID); err == nil {
		t.Fatal("missing recovery ledger disclosed invitation")
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = checkEnrollmentRecovery(ctx, tx); err == nil {
		t.Fatal("missing ledger admitted transaction")
	}
}
