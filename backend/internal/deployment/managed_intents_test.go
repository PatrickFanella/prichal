package deployment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func managedIntentFixture(t *testing.T) (*Store, string, string) {
	t.Helper()
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
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('fixture','managed-intents','Managed intents') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Managed intents',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := (entitlement.Store{Pool: pool}).Submit(ctx, "test-operator", org, entitlement.Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", State: "active", Plan: "local-small", Hours: 24, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	store.Staging, store.ManagedIntents = true, true
	return store, owner, org
}

func assertIntentCounts(t *testing.T, store *Store, deployments, reservations, events, jobs int) {
	t.Helper()
	var d, r, e, j int
	if err := store.Pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM deployments),(SELECT count(*) FROM managed_public_endpoints),(SELECT count(*) FROM deployment_events),(SELECT count(*) FROM river_job)`).Scan(&d, &r, &e, &j); err != nil {
		t.Fatal(err)
	}
	if d != deployments || r != reservations || e != events || j != jobs {
		t.Fatalf("partial intent: deployments=%d reservations=%d events=%d jobs=%d", d, r, e, j)
	}
}

func TestManagedIntentReplayPreservesPublicAndLegacyIdentity(t *testing.T) {
	s, owner, org := managedIntentFixture(t)
	ctx := context.Background()
	s.ManagedIntents = false
	legacyRequest := Request{Kind: "agent", Name: "private", Region: "local", Key: "private"}
	legacy, err := s.Submit(ctx, owner, org, legacyRequest)
	if err != nil {
		t.Fatal(err)
	}
	s.ManagedIntents = true
	retained, err := s.Submit(ctx, owner, org, legacyRequest)
	if err != nil || retained.ID != legacy.ID || retained.ManagedHostname != legacy.ManagedHostname || retained.Endpoint != legacy.Endpoint {
		t.Fatal("mode change converted retained private intent", err)
	}
	r := Request{Kind: "agent", Name: "managed", Region: "local", Key: "managed"}
	public, err := s.Submit(ctx, owner, org, r)
	if err != nil {
		t.Fatal(err)
	}
	host, err := managednamespace.Endpoint(public.ID)
	if err != nil || public.ManagedHostname != host || public.Endpoint != "" || public.State != "queued" || public.Message != "Managed hostname reserved. Public activation is pending." {
		t.Fatal("managed intent misrepresented serving", public, err)
	}
	var recordedHost, actor, basis, queue string
	var port int
	if err := s.Pool.QueryRow(ctx, `SELECT hostname,forward_port,actor,reservation_basis FROM managed_public_endpoints WHERE deployment_id=$1`, public.ID).Scan(&recordedHost, &port, &actor, &basis); err != nil {
		t.Fatal(err)
	}
	if recordedHost != host || port != 35000 || actor != "owner:"+owner || basis != "queued" {
		t.Fatal("reservation differs from owner intent")
	}
	if err := s.Pool.QueryRow(ctx, `SELECT queue FROM river_job WHERE args->>'deploymentId'=$1`, public.ID).Scan(&queue); err != nil || queue != ManagedIntentQueue {
		t.Fatal("managed intent escaped staging queue", err)
	}
	s.ManagedIntents = false
	if _, err := s.SetAdmission(ctx, "test-operator", "maintenance", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE pilot_entitlement_operations SET expires_at=now()-interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	retained, err = s.Submit(ctx, owner, org, r)
	if err != nil || retained.ID != public.ID || retained.ManagedHostname != host || retained.Endpoint != "" {
		t.Fatal("replay changed managed intent after gate changes", err)
	}
	r.Name = "conflict"
	if _, err := s.Submit(ctx, owner, org, r); !errors.Is(err, ErrConflict) {
		t.Fatal("managed request key accepted changed payload", err)
	}
	assertIntentCounts(t, s, 2, 1, 2, 2)
}

func TestManagedIntentFailuresRollbackEveryAuthorityRow(t *testing.T) {
	for _, failure := range []string{"reservation", "job", "recovery"} {
		t.Run(failure, func(t *testing.T) {
			s, owner, org := managedIntentFixture(t)
			ctx := context.Background()
			query := map[string]string{
				"reservation": `ALTER TABLE managed_public_endpoints ADD CONSTRAINT reject_reservations CHECK(actor='refused') NOT VALID`,
				"job":         `ALTER TABLE river_job ADD CONSTRAINT reject_jobs CHECK(kind<>'deploy') NOT VALID`,
				"recovery":    `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`,
			}[failure]
			if _, err := s.Pool.Exec(ctx, query); err != nil {
				t.Fatal(err)
			}
			r := Request{Kind: "agent", Name: "rollback", Region: "local", Key: "rollback"}
			if _, err := s.Submit(ctx, owner, org, r); err == nil {
				t.Fatal("refused transaction succeeded")
			}
			assertIntentCounts(t, s, 0, 0, 0, 0)
			if failure != "recovery" {
				repair := map[string]string{"reservation": `ALTER TABLE managed_public_endpoints DROP CONSTRAINT reject_reservations`, "job": `ALTER TABLE river_job DROP CONSTRAINT reject_jobs`}[failure]
				if _, err := s.Pool.Exec(ctx, repair); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Submit(ctx, owner, org, r); err != nil {
					t.Fatal("same request failed after repair", err)
				}
				assertIntentCounts(t, s, 1, 1, 1, 1)
			}
		})
	}
}

type managedForbiddenAgent struct{ calls int }

func (a *managedForbiddenAgent) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	a.calls++
	return nodeagent.Capacity{}, errors.New("unexpected host call")
}
func (a *managedForbiddenAgent) Ensure(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	a.calls++
	return nodeagent.Instance{}, errors.New("unexpected host call")
}

func TestManagedIntentWorkerCannotActivatePrivateFallback(t *testing.T) {
	s, owner, org := managedIntentFixture(t)
	ctx := context.Background()
	op, err := s.Submit(ctx, owner, org, Request{Kind: "agent", Name: "pending", Region: "local", Key: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	a := &managedForbiddenAgent{}
	worker := Worker{Pool: s.Pool, Provisioner: Router{Pool: s.Pool, Agent: a, AgentOnly: true}}
	job := &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3}, Args: Args{op.ID}}
	for _, query := range []string{
		`SELECT 1`,
		`UPDATE deployments SET managed_hostname=id::text||'.localhost'`,
		`UPDATE deployments SET managed_hostname='pds-'||replace(id::text,'-','')||'.subcult.tv'; DELETE FROM managed_public_endpoints`,
	} {
		if _, err := s.Pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
		var snooze *river.JobSnoozeError
		if err := worker.Work(ctx, job); !errors.As(err, &snooze) || a.calls != 0 {
			t.Fatal("public intent executed host work or exhausted its retained retry", err, a.calls)
		}
	}
	retained, err := s.Get(ctx, owner, op.ID)
	if err != nil || retained.State != "queued" || retained.Endpoint != "" || len(retained.Events) != 1 {
		t.Fatal("pending activation advanced", err)
	}
	var placed int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements`).Scan(&placed); err != nil || placed != 0 {
		t.Fatal("pending activation placed native resources", err)
	}
}

func TestConcurrentManagedIntentReplaysCommitOneReservation(t *testing.T) {
	s, owner, org := managedIntentFixture(t)
	r := Request{Kind: "agent", Name: "concurrent", Region: "local", Key: "concurrent"}
	var wg sync.WaitGroup
	results := make(chan Operation, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op, err := s.Submit(context.Background(), owner, org, r)
			results <- op
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id string
	for op := range results {
		if id != "" && id != op.ID {
			t.Fatal("concurrent retry created a second identity")
		}
		id = op.ID
	}
	assertIntentCounts(t, s, 1, 1, 1, 1)
}

func TestManagedIntentRechecksRecoveryAfterJobInsertionWait(t *testing.T) {
	s, owner, org := managedIntentFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := s.Pool.Exec(ctx, `CREATE FUNCTION managed_intent_wait() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(73647305); RETURN NEW; END $$; CREATE TRIGGER managed_intent_wait BEFORE INSERT ON river_job FOR EACH ROW EXECUTE FUNCTION managed_intent_wait()`); err != nil {
		t.Fatal(err)
	}
	blocker, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_xact_lock(73647305)`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.Submit(ctx, owner, org, Request{Kind: "agent", Name: "late-hold", Region: "local", Key: "late-hold"})
		done <- err
	}()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var waiting bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=73647305 AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatal("intent did not reach the controlled job-insert wait", err)
		case <-ctx.Done():
			t.Fatal("job insertion did not wait", ctx.Err())
		case <-tick.C:
		}
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, recovery.ErrHeld) {
		t.Fatal("late hold allowed public intent commit", err)
	}
	assertIntentCounts(t, s, 0, 0, 0, 0)
}
