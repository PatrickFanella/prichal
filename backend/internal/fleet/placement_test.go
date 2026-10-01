package fleet

import (
	"context"
	"errors"
	"sync"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestCapacityReservations(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, node string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','alice','Alice') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Studio',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,last_verified_at) VALUES('local','https://127.0.0.1:9091',1,1,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 2)
	for i := range ids {
		if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key) VALUES($1,$2,'local',$2) RETURNING id`, org, []string{"one", "two"}[i]).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	store := Store{Pool: pool}
	var wg sync.WaitGroup
	results := make(chan string, 2)
	failures := make(chan error, 2)
	for _, id := range ids {
		wg.Go(func() {
			if _, err := store.Reserve(ctx, id); err != nil {
				failures <- err
			} else {
				results <- id
			}
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	if len(results) != 1 || len(failures) != 1 {
		t.Fatalf("oversubscription or lost capacity: %d success %d failures", len(results), len(failures))
	}
	for err := range failures {
		if !errors.Is(err, ErrCapacity) {
			t.Fatal(err)
		}
	}
	successful := <-results
	if err := store.RecordCapacity(ctx, node, 1, 8, 8<<30, []string{successful, "cccccccc-cccc-cccc-cccc-cccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	var measuredSlots int
	if err := pool.QueryRow(ctx, `SELECT slots FROM hosting_nodes WHERE id=$1`, node).Scan(&measuredSlots); err != nil || measuredSlots != 0 {
		t.Fatalf("retained external namespace ignored: %d %v", measuredSlots, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE hosting_nodes SET state='draining' WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	if replay, err := store.Reserve(ctx, successful); err != nil || replay.NodeID != node {
		t.Fatalf("existing placement changed: %v %+v", err, replay)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("reservations: %d %v", count, err)
	}
}
