package enrollment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

func acceptedRemovalMember(t *testing.T, f storeFixture, n int, did string) MemberInvitation {
	t.Helper()
	ctx := context.Background()
	m, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(n))
	if err != nil {
		t.Fatal(err)
	}
	a := &memberAgentFixture{state: "accepted", did: did, handle: "member.test"}
	if err = (&MemberWorker{Pool: f.pool, Cipher: f.store.Cipher, Queue: f.store.Queue, Agent: a}).Work(ctx, memberJob(m.ID, 1)); err != nil {
		t.Fatal(err)
	}
	m, err = f.store.GetMember(ctx, f.owner, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func removalJob(id string, attempt int) *river.Job[MemberRemovalArgs] {
	return &river.Job[MemberRemovalArgs]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 5}, Args: MemberRemovalArgs{id}}
}

type removalAgentFixture struct {
	result  nodeagent.MemberExit
	calls   int
	failure bool
}

func (a *removalAgentFixture) MemberExit(_ context.Context, _ nodeagent.Peer, id, ticket string) (nodeagent.MemberExit, error) {
	a.calls++
	if a.failure {
		return nodeagent.MemberExit{}, errors.New("PRIVATE NATIVE PASSWORD")
	}
	r := a.result
	r.InstanceID = id
	r.Ticket = ticket
	return r, nil
}
func removalWindow(did string, start time.Time) nodeagent.MemberExit {
	return nodeagent.MemberExit{DID: did, State: "migration_window", RequestedAt: start.UnixMilli(), DeactivateAfter: start.Add(nodeagent.MemberMigrationWindow).UnixMilli()}
}
func requireRemovalSnooze(t *testing.T, err error) {
	t.Helper()
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("expected retained scheduled work: %v", err)
	}
}

func TestMemberRemovalIntentIsAtomicScopedAndIdempotent(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 900, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	key := memberKey(901)
	if _, err := f.store.RemoveMember(ctx, f.bob, m.ID, key); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign actor removed member", err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.store.RemoveMember(ctx, f.owner, m.ID, key)
			if e != nil {
				errs <- e
			} else {
				ids <- r.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	id := ""
	for got := range ids {
		if id == "" {
			id = got
		}
		if got != id {
			t.Fatal("duplicate removal")
		}
	}
	var rows, jobs, events int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM member_removals),(SELECT count(*) FROM river_job WHERE kind='member_removal'),(SELECT count(*) FROM member_removal_events)`).Scan(&rows, &jobs, &events); err != nil || rows != 1 || jobs != 1 || events != 1 {
		t.Fatalf("atomic request %d/%d/%d: %v", rows, jobs, events, err)
	}
	r, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || r.State != "requested" || r.DeactivateAfter != nil || r.RequestedBy != f.owner || r.DID != m.DID || r.Events[0].ActorID == nil || *r.Events[0].ActorID != f.owner {
		t.Fatalf("request receipt: %+v %v", r, err)
	}
	if _, err = f.store.RemoveMember(ctx, f.owner, m.ID, memberKey(902)); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay key accepted", err)
	}
	other := acceptedRemovalMember(t, f, 903, "did:plc:cccccccccccccccccccccccc")
	if _, err = f.store.RemoveMember(ctx, f.owner, other.ID, key); !errors.Is(err, ErrConflict) {
		t.Fatal("key rebound to other account", err)
	}
	pending, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(904))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.RemoveMember(ctx, f.owner, pending.ID, memberKey(905)); !errors.Is(err, ErrConflict) {
		t.Fatal("pending invitation accepted as removal", err)
	}
	if _, err = f.store.GetMemberRemoval(ctx, f.bob, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign actor read removal", err)
	}
}

func TestMemberRemovalAdministratorAttributionAndRevocation(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 910, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	if _, err := f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT organization_id,$1,'administrator','active' FROM deployments WHERE id=$2`, f.bob, f.deployment); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RemoveMember(ctx, f.bob, m.ID, memberKey(911)); !errors.Is(err, ErrNotFound) {
		t.Fatal("administrator bypassed opt-in")
	}
	f.store.MemberAdministrators = true
	r, err := f.store.RemoveMember(ctx, f.bob, m.ID, memberKey(911))
	if err != nil || r.RequestedBy != f.bob || r.Events[0].ActorID == nil || *r.Events[0].ActorID != f.bob {
		t.Fatalf("administrator attribution: %+v %v", r, err)
	}
	var owner string
	if err = f.pool.QueryRow(ctx, `SELECT owner_id::text FROM member_removals WHERE id=$1`, r.ID).Scan(&owner); err != nil || owner != f.owner {
		t.Fatal("canonical owner changed", err)
	}
	for _, state := range []string{"revoked", "active"} {
		role := "administrator"
		if state == "active" {
			role = "viewer"
		}
		if _, err = f.pool.Exec(ctx, `UPDATE organization_members SET state=$1,role=$2 WHERE owner_id=$3`, state, role, f.bob); err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.GetMemberRemoval(ctx, f.bob, m.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("revoked or viewer actor read removal")
		}
		if _, err = f.store.RemoveMember(ctx, f.bob, m.ID, memberKey(911)); !errors.Is(err, ErrNotFound) {
			t.Fatal("revoked or viewer actor replayed removal")
		}
	}
	// Revocation blocks new manager actions, but does not cancel already-authorized work.
	a := &removalAgentFixture{result: removalWindow(m.DID, time.Now())}
	requireRemovalSnooze(t, (&MemberRemovalWorker{Pool: f.pool, Agent: a}).Work(ctx, removalJob(r.ID, 1)))
}

func TestMemberRemovalWorkerRetainsWindowAndConfirmsCompletion(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 920, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	r, err := f.store.RemoveMember(ctx, f.owner, m.ID, memberKey(921))
	if err != nil {
		t.Fatal(err)
	}
	a := &removalAgentFixture{result: removalWindow(m.DID, time.Now().UTC().Truncate(time.Millisecond))}
	w := &MemberRemovalWorker{Pool: f.pool, Agent: a}
	requireRemovalSnooze(t, w.Work(ctx, removalJob(r.ID, 1)))
	first, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || first.State != "migration_window" || first.DeactivateAfter == nil {
		t.Fatalf("window: %+v %v", first, err)
	}
	w = &MemberRemovalWorker{Pool: f.pool, Agent: a}
	requireRemovalSnooze(t, w.Work(ctx, removalJob(r.ID, 2)))
	if a.calls != 1 {
		t.Fatal("worker touched native account before deadline")
	}
	// Simulate an elapsed window in this disposable management database, without a real 30-day soak.
	old := time.Now().Add(-31 * 24 * time.Hour).UTC().Truncate(time.Millisecond)
	a.result = removalWindow(m.DID, old)
	if _, err = f.pool.Exec(ctx, `UPDATE member_removals SET created_at=$2,host_requested_at=$2,deactivate_after=$3 WHERE id=$1`, r.ID, old, old.Add(nodeagent.MemberMigrationWindow)); err != nil {
		t.Fatal(err)
	}
	a.result.DID = "did:plc:cccccccccccccccccccccccc"
	if err = w.Work(ctx, removalJob(r.ID, 2)); err == nil {
		t.Fatal("foreign native completion accepted")
	}
	a.result = removalWindow(m.DID, old.Add(time.Hour))
	if err = w.Work(ctx, removalJob(r.ID, 2)); err == nil {
		t.Fatal("changed native deadline accepted")
	}
	a.result = removalWindow(m.DID, old)
	a.result.State = "deactivated"
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_removal_completion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='deactivated' THEN RAISE EXCEPTION 'PRIVATE COMPLETION'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_removal_completion BEFORE UPDATE ON member_removals FOR EACH ROW EXECUTE FUNCTION fail_removal_completion()`); err != nil {
		t.Fatal(err)
	}
	if err = w.Work(ctx, removalJob(r.ID, 2)); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("uncommitted completion leaked or succeeded", err)
	}
	retained, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || retained.State != "migration_window" || retained.CompletedAt != nil || retained.DeactivateAfter.UnixMilli() != a.result.DeactivateAfter {
		t.Fatal("failed completion lost original window", err)
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_removal_completion ON member_removals; DROP FUNCTION fail_removal_completion()`); err != nil {
		t.Fatal(err)
	}
	if err = w.Work(ctx, removalJob(r.ID, 3)); err != nil {
		t.Fatal(err)
	}
	calls := a.calls
	if err = w.Work(ctx, removalJob(r.ID, 4)); err != nil || a.calls != calls {
		t.Fatal("completion replay touched native account", err)
	}
	done, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || done.State != "deactivated" || done.CompletedAt == nil || len(done.Events) != 3 || done.Events[1].ActorID != nil || done.Events[2].ActorID != nil {
		t.Fatalf("completion audit: %+v %v", done, err)
	}
	summary, err := f.store.ListMembers(ctx, f.owner, f.deployment)
	if err != nil || summary.Members != 1 || len(summary.Accounts) != 1 {
		t.Fatal("deactivation erased accepted identity or freed quota", err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE member_removals SET deactivate_after=deactivate_after-interval '1 second' WHERE id=$1`, r.ID); err == nil {
		t.Fatal("database accepted shortened window")
	}
}

func TestMemberRemovalUncertainResultReviewAndAtomicCheckpoint(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 930, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	key := memberKey(931)
	r, err := f.store.RemoveMember(ctx, f.owner, m.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	a := &removalAgentFixture{result: removalWindow(m.DID, time.Now().Truncate(time.Millisecond))}
	w := &MemberRemovalWorker{Pool: f.pool, Agent: a}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_member_removal() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='migration_window' THEN RAISE EXCEPTION 'PRIVATE CHECKPOINT'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_member_removal BEFORE UPDATE ON member_removals FOR EACH ROW EXECUTE FUNCTION fail_member_removal()`); err != nil {
		t.Fatal(err)
	}
	if err = w.Work(ctx, removalJob(r.ID, 1)); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("uncommitted checkpoint leaked or succeeded", err)
	}
	unchanged, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || unchanged.State != "requested" || len(unchanged.Events) != 1 {
		t.Fatal("failed checkpoint partially committed", err)
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_member_removal ON member_removals; DROP FUNCTION fail_member_removal()`); err != nil {
		t.Fatal(err)
	}
	a.failure = true
	if err = w.Work(ctx, removalJob(r.ID, 5)); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("native failure leaked or succeeded", err)
	}
	review, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || review.State != "review" || review.DeactivateAfter != nil {
		t.Fatal("uncertain result claimed native outcome", err)
	}
	retried, err := f.store.RemoveMember(ctx, f.owner, m.ID, key)
	if err != nil || retried.ID != r.ID || retried.State != "requested" {
		t.Fatal("review retry lost retained intent", err)
	}
	a.failure = false
	requireRemovalSnooze(t, w.Work(ctx, removalJob(r.ID, 1)))
	recovered, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || recovered.HostRequestedAt.UnixMilli() != a.result.RequestedAt {
		t.Fatal("reconciliation changed native window", err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
		t.Fatal(err)
	}
	calls := a.calls
	requireRemovalSnooze(t, w.Work(ctx, removalJob(r.ID, 1)))
	if a.calls != calls {
		t.Fatal("recovery hold reached agent")
	}
	if _, err = f.store.RemoveMember(ctx, f.owner, m.ID, key); err == nil {
		t.Fatal("recovery hold admitted replay")
	}
}

func TestMemberRemovalMigrationPreservesAcceptedAccounts(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 940, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	if _, err := f.pool.Exec(ctx, `DROP TABLE member_removal_events,member_removals;UPDATE schema_version SET version=37`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Apply(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	if err := migrate.Apply(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	kept, err := f.store.GetMember(ctx, f.owner, m.ID)
	if err != nil || kept.State != "accepted" || kept.DID != m.DID {
		t.Fatal("migration changed accepted account", err)
	}
	if _, err = f.store.GetMemberRemoval(ctx, f.owner, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("migration invented a removal", err)
	}
}

func TestMemberRemovalQueueFailureRollsBackIntentAndActor(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 960, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION fail_removal_queue() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='member_removal' THEN RAISE EXCEPTION 'queue unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_removal_queue BEFORE INSERT ON river_job FOR EACH ROW EXECUTE FUNCTION fail_removal_queue()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RemoveMember(ctx, f.owner, m.ID, memberKey(961)); err == nil {
		t.Fatal("unqueued removal claimed success")
	}
	var intents, events int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM member_removals),(SELECT count(*) FROM member_removal_events)`).Scan(&intents, &events); err != nil || intents != 0 || events != 0 {
		t.Fatalf("partial intent/audit %d/%d: %v", intents, events, err)
	}
}

func TestMemberRemovalWorkerRefusesRuntimeOwnershipAndIdentityDrift(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 970, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	r, err := f.store.RemoveMember(ctx, f.owner, m.ID, memberKey(971))
	if err != nil {
		t.Fatal(err)
	}
	a := &removalAgentFixture{result: removalWindow(m.DID, time.Now())}
	w := &MemberRemovalWorker{Pool: f.pool, Agent: a}
	for _, sql := range []string{
		`UPDATE deployments SET state='failed'`,
		`UPDATE deployments SET state='ready'; UPDATE organizations SET owner_id=(SELECT id FROM owners WHERE subject='bob')`,
		`UPDATE organizations SET owner_id=(SELECT id FROM owners WHERE subject='alice'); UPDATE member_invitations SET account_did='did:plc:cccccccccccccccccccccccc' WHERE state='accepted'`,
	} {
		if _, err = f.pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
		if err = w.Work(ctx, removalJob(r.ID, 1)); err == nil || a.calls != 0 {
			t.Fatal("unsafe retained scope reached native agent", err)
		}
	}
	if _, err = f.pool.Exec(ctx, `UPDATE member_invitations SET account_did=$1 WHERE id=$2`, m.DID, m.ID); err != nil {
		t.Fatal(err)
	}
	a.result.State = "deactivated"
	if err = w.Work(ctx, removalJob(r.ID, 1)); err == nil {
		t.Fatal("premature native completion accepted")
	}
	kept, err := f.store.GetMemberRemoval(ctx, f.owner, m.ID)
	if err != nil || kept.State != "requested" || kept.CompletedAt != nil {
		t.Fatal("unconfirmed completion changed receipt", err)
	}
	other := acceptedRemovalMember(t, f, 972, "did:plc:cccccccccccccccccccccccc")
	var export string
	if err = f.pool.QueryRow(ctx, `INSERT INTO export_operations(deployment_id,owner_id,idempotency_key,account_did) VALUES($1,$2,gen_random_uuid(),'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa') RETURNING id::text`, f.deployment, f.owner).Scan(&export); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO hosting_exits(deployment_id,owner_id,idempotency_key,export_id,service_end_at,retention_review_at,delay_seconds,retention_days) VALUES($1,$2,gen_random_uuid(),$3,date_trunc('second',now())+interval '1 hour',date_trunc('second',now())+interval '25 hours',3600,1)`, f.deployment, f.owner, export); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.RemoveMember(ctx, f.owner, other.ID, memberKey(973)); !errors.Is(err, ErrHostingExit) {
		t.Fatal("hosting exit admitted a new removal", err)
	}
	calls := a.calls
	a.result.State = "migration_window"
	if err = w.Work(ctx, removalJob(r.ID, 1)); err == nil || a.calls != calls {
		t.Fatal("hosting exit reached native member agent", err)
	}
}

func TestMemberRemovalRiverPersistsFutureReconciliation(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	m := acceptedRemovalMember(t, f, 980, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	if _, err := f.store.RemoveMember(ctx, f.owner, m.ID, memberKey(981)); err != nil {
		t.Fatal(err)
	}
	// The acceptance helper performed its invitation job synchronously above.
	if _, err := f.pool.Exec(ctx, `UPDATE river_job SET state='cancelled',finalized_at=now() WHERE kind='member_invitation'`); err != nil {
		t.Fatal(err)
	}
	a := &removalAgentFixture{result: removalWindow(m.DID, time.Now().UTC().Truncate(time.Millisecond))}
	workers := river.NewWorkers()
	river.AddWorker(workers, &MemberRemovalWorker{Pool: f.pool, Agent: a})
	client, err := river.NewClient(riverpgxv5.New(f.pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Stop(stop); err != nil {
			t.Error(err)
		}
	})
	until := time.Now().Add(10 * time.Second)
	var scheduled time.Time
	for {
		err = f.pool.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind='member_removal' AND state='scheduled'`).Scan(&scheduled)
		if err == nil {
			break
		}
		if time.Now().After(until) {
			t.Fatal("River did not persist future member reconciliation", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	deadline := time.UnixMilli(a.result.DeactivateAfter)
	if scheduled.Before(deadline.Add(-time.Second)) || scheduled.After(deadline.Add(time.Second)) {
		t.Fatalf("durable schedule %s differs from host deadline %s", scheduled, deadline)
	}
	stop, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = client.Stop(stop); err != nil {
		t.Fatal(err)
	}
	if a.calls != 1 {
		t.Fatal("River repeated native dispatch before due time")
	}
}
