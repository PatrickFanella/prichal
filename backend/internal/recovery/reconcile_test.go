package recovery

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type observerFixture struct {
	ids         []string
	healthy     bool
	unavailable bool
}

func (o observerFixture) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	if o.unavailable {
		return nodeagent.Capacity{}, errors.New("unavailable")
	}
	return nodeagent.Capacity{InstanceIDs: o.ids}, nil
}
func (o observerFixture) Status(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: o.healthy}, nil
}
func TestReconciliationPreservesHoldOnAmbiguity(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','recovery','Recovery') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Recovery',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,'recovery','local','recovery','agent','checking','recovery.localhost','https://recovery.localhost:44444') RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []observerFixture{
		{ids: []string{id}, healthy: true, unavailable: true},
		{healthy: true},
		{ids: []string{id}, healthy: false},
		{ids: []string{id, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, healthy: true},
		{ids: []string{id, id}, healthy: true},
	} {
		if err := Reconcile(ctx, pool, fixture); err == nil {
			t.Fatal("ambiguous inventory released")
		}
		if err := Check(ctx, pool); !errors.Is(err, ErrHeld) {
			t.Fatalf("failed reconciliation changed hold: %v", err)
		}
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, healthy: true}); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM deployments WHERE id=$1`, id).Scan(&state); err != nil || state != "checking" {
		t.Fatal("reconciliation advanced deployment")
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, healthy: true}); err == nil {
		t.Fatal("reconciliation ran without hold")
	}
}
