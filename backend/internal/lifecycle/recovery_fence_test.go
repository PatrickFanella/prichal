package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

func lifecycleRecoveryFixture(t *testing.T) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','lifecycle-recovery','Recovery') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Recovery',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,'recovery','local','recovery','agent','ready','recovery.localhost') RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	return store, pool, id
}

func TestLifecycleHoldDuringDeploymentLockWait(t *testing.T) {
	for _, tc := range []struct{ mode, ledger string }{{"new", "held"}, {"retry", "held"}, {"worker", "held"}, {"new", "missing"}, {"retry", "missing"}, {"worker", "missing"}} {
		t.Run(tc.mode+"-"+tc.ledger, func(t *testing.T) {
			mode := tc.mode
			store, pool, id := lifecycleRecoveryFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request := Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", Kind: "service", State: "suspended", Reason: "maintenance"}
			var operation Operation
			if mode != "new" {
				var err error
				operation, err = store.Submit(ctx, "local-operator", id, request)
				if err != nil {
					t.Fatal(err)
				}
			}
			inventory := func() string { return lifecycleRecoveryInventory(t, pool) }
			before := inventory()
			block, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer block.Rollback(context.Background())
			if _, err := block.Exec(ctx, `SELECT id FROM deployments WHERE id=$1 FOR UPDATE`, id); err != nil {
				t.Fatal(err)
			}
			agent := &fakeAgent{}
			done := make(chan error, 1)
			go func() {
				if mode == "worker" {
					done <- (&Worker{Pool: pool, Agent: agent}).Work(ctx, lifecycleJob(operation.ID, 3))
				} else {
					_, err := store.Submit(ctx, "local-operator", id, request)
					done <- err
				}
			}()
			for {
				var waiting bool
				if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM deployments WHERE id=%FOR UPDATE%')`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("lifecycle did not reach deployment lock")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if tc.ledger == "held" {
				if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
					t.Fatal(err)
				}
			} else if _, err := pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
				t.Fatal(err)
			}
			if err := block.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			var result error
			select {
			case result = <-done:
			case <-ctx.Done():
				t.Fatal("lifecycle remained blocked")
			}
			if mode != "worker" && (result == nil || tc.ledger == "held" && !errors.Is(result, recovery.ErrHeld)) {
				t.Fatal("held lifecycle submission accepted", result)
			}
			var snooze *river.JobSnoozeError
			if mode == "worker" && (!errors.As(result, &snooze) || agent.calls != 0) {
				t.Fatal("held lifecycle worker dispatched host action", agent.calls)
			}
			if inventory() != before {
				t.Fatal("held lifecycle changed intent, audit or jobs")
			}
		})
	}
}

func TestLifecycleHoldDuringTerminalAuditWaitPreservesPendingIntent(t *testing.T) {
	store, pool, id := lifecycleRecoveryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	op, err := store.Submit(ctx, "local-operator", id, Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", Kind: "service", State: "suspended", Reason: "maintenance"})
	if err != nil {
		t.Fatal(err)
	}
	before := lifecycleRecoveryInventory(t, pool)
	block, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback(context.Background())
	if _, err := block.Exec(ctx, `LOCK TABLE service_operation_events IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	agent := &fakeAgent{fail: true}
	done := make(chan error, 1)
	go func() { done <- (&Worker{Pool: pool, Agent: agent}).Work(ctx, lifecycleJob(op.ID, 3)) }()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'INSERT INTO service_operation_events%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal audit did not reach table lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := block.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var result error
	select {
	case result = <-done:
	case <-ctx.Done():
		t.Fatal("worker remained blocked")
	}
	var snooze *river.JobSnoozeError
	if !errors.As(result, &snooze) || agent.calls != 1 {
		t.Fatal("held failure was not snoozed after the previously authorized attempt")
	}
	if lifecycleRecoveryInventory(t, pool) != before {
		t.Fatal("held terminal failure changed retained intent, audit or job")
	}
}

func lifecycleRecoveryInventory(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var value string
	if err := pool.QueryRow(context.Background(), `SELECT json_build_object('operations',(SELECT coalesce(json_agg(o ORDER BY id),'[]') FROM service_operations o),'events',(SELECT coalesce(json_agg(e ORDER BY id),'[]') FROM service_operation_events e),'jobs',(SELECT coalesce(json_agg(j ORDER BY id),'[]') FROM river_job j))::text`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestLifecycleHoldDuringNewIntentAuditWaitRollsBackQueue(t *testing.T) {
	store, pool, id := lifecycleRecoveryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	before := lifecycleRecoveryInventory(t, pool)
	block, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback(context.Background())
	if _, err := block.Exec(ctx, `LOCK TABLE service_operation_events IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.Submit(ctx, "local-operator", id, Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", Kind: "service", State: "suspended", Reason: "maintenance"})
		done <- err
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'INSERT INTO service_operation_events%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("intent audit did not reach table lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := block.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
	case <-ctx.Done():
		t.Fatal("intent remained blocked")
	}
	if !errors.Is(err, recovery.ErrHeld) || lifecycleRecoveryInventory(t, pool) != before {
		t.Fatal("held intent persisted an operation, audit or queued job", err)
	}
}
