package deployment

import (
	"context"
	"errors"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestDurableIntent(t *testing.T) {
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
	request := Request{Name: "studio", Region: "local", Key: "request-1"}
	first, err := store.Submit(ctx, owner, org, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Submit(ctx, owner, org, request)
	if err != nil || second.ID != first.ID {
		t.Fatalf("replay: %v %+v", err, second)
	}
	request.Name = "different"
	if _, err = store.Submit(ctx, owner, org, request); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
	if _, err = store.Get(ctx, "00000000-0000-0000-0000-000000000000", first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign owner: %v", err)
	}
	var jobs int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("jobs: %d %v", jobs, err)
	}
	// Reconstruct the worker between steps, as after a process restart.
	for i := 0; i < 3; i++ {
		worker := TestProvisioner{Pool: pool}
		if err = worker.Step(ctx, first.ID); err != nil {
			t.Fatal(err)
		}
	}
	worker := TestProvisioner{Pool: pool}
	if err = worker.Step(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	result, err := store.Get(ctx, owner, first.ID)
	if err != nil || result.State != "ready" || len(result.Events) != 4 {
		t.Fatalf("result: %+v %v", result, err)
	}
	var instances int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM test_instances`).Scan(&instances); err != nil || instances != 1 {
		t.Fatalf("instances: %d %v", instances, err)
	}
	// Force job insertion to fail and prove intent/event rollback.
	if _, err = pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT reject_jobs CHECK(kind <> 'deploy') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	request.Name = "rollback"
	request.Key = "request-2"
	if _, err = store.Submit(ctx, owner, org, request); err == nil {
		t.Fatal("insert unexpectedly succeeded")
	}
	var intents int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM deployments`).Scan(&intents); err != nil || intents != 1 {
		t.Fatalf("partial intent: %d %v", intents, err)
	}
}

func TestRiverWorkerCompletesPersistedJob(t *testing.T) {
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
	op, err := store.Submit(ctx, owner, org, Request{"studio", "local", "persisted"})
	if err != nil {
		t.Fatal(err)
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &Worker{Pool: pool, Provisioner: TestProvisioner{Pool: pool}})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}}, TestOnly: true})
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
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var complete bool
		if err = pool.QueryRow(ctx, `SELECT state='completed' FROM river_job`).Scan(&complete); err != nil {
			t.Fatal(err)
		}
		if complete {
			result, err := store.Get(ctx, owner, op.ID)
			if err != nil || result.State != "ready" {
				t.Fatalf("completed job without ready operation: %+v %v", result, err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("persisted River job did not complete")
}

type unavailableProvisioner struct{}

func (unavailableProvisioner) Step(context.Context, string) error {
	return errors.New("private host credential must not reach customer")
}
func TestBoundedFailure(t *testing.T) {
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
	op, err := store.Submit(ctx, owner, org, Request{"failure", "local", "failure"})
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{Pool: pool, Provisioner: unavailableProvisioner{}}
	for attempt := 1; attempt <= 3; attempt++ {
		job := &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 3}, Args: Args{op.ID}}
		err = worker.Work(ctx, job)
		if err == nil || strings.Contains(err.Error(), "credential") {
			t.Fatalf("unsafe error: %v", err)
		}
		result, err := store.Get(ctx, owner, op.ID)
		if err != nil {
			t.Fatal(err)
		}
		expected := "queued"
		if attempt == 3 {
			expected = "failed"
		}
		if result.State != expected {
			t.Fatalf("attempt %d: %s", attempt, result.State)
		}
	}
}
