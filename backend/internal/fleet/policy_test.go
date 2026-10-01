package fleet

import (
	"context"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"testing"
)

func TestCapacityPolicyAuditAndFreshMeasurementFence(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var node string
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,last_verified_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id::text`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	store := Store{Pool: pool}
	for _, entry := range []struct {
		actor, reason string
		ceiling       int
	}{{"", "planned_capacity_change", 3}, {"operator", "private unreviewed text", 3}, {"operator", "planned_capacity_change", 9}, {"operator", "planned_capacity_change", 0}} {
		if err := store.SetCapacityCeiling(ctx, node, entry.actor, entry.reason, entry.ceiling); err == nil {
			t.Fatal("invalid operator policy accepted")
		}
	}
	if err := store.SetCapacityCeiling(ctx, node, "operator", "local_isolation_qualification", 3); err != nil {
		t.Fatal(err)
	}
	var ceiling, slots int
	var fresh bool
	if err := pool.QueryRow(ctx, `SELECT operator_slots,slots,last_verified_at IS NOT NULL FROM hosting_nodes WHERE id=$1`, node).Scan(&ceiling, &slots, &fresh); err != nil || ceiling != 3 || slots != 2 || fresh {
		t.Fatal("policy expanded unmeasured placement")
	}
	if err := store.SetCapacityCeiling(ctx, node, "operator", "local_isolation_qualification", 3); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM hosting_node_policy_events WHERE previous_slots=2 AND requested_slots=3 AND actor='operator'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("operator policy audit or replay failed")
	}
	if err := store.RecordCapacity(ctx, node, 2, 8, 8<<30, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT slots FROM hosting_nodes WHERE id=$1`, node).Scan(&slots); err != nil || slots != 2 {
		t.Fatal("measurement ignored agent ceiling")
	}
	if err := store.SetCapacityCeiling(ctx, node, "operator", "capacity_reduction", 1); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT slots FROM hosting_nodes WHERE id=$1`, node).Scan(&slots); err != nil || slots != 1 {
		t.Fatal("capacity reduction ignored")
	}
	if _, err := pool.Exec(ctx, `UPDATE hosting_nodes SET state='pending' WHERE id=$1`, node); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCapacityCeiling(ctx, node, "operator", "planned_capacity_change", 2); err == nil {
		t.Fatal("unregistered node policy accepted")
	}
}
