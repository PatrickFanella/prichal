package deployment

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/fleet"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type remoteAgent struct {
	ids             map[string]bool
	failAfterCreate bool
	healthy         bool
}

func (p *remoteAgent) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{MaximumSlots: 2, CPUs: 8, MemoryBytes: 8 << 30, InstanceIDs: []string{}}, nil
}
func (p *remoteAgent) Ensure(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	p.ids[id] = true
	if p.failAfterCreate {
		p.failAfterCreate = false
		return nodeagent.Instance{}, errors.New("connection lost after host success")
	}
	return nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: p.healthy}, nil
}
func TestAgentSuccessAfterUncertainCompletion(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','alice','Alice') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Studio',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1)`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (entitlement.Store{Pool: pool}).Submit(ctx, "test-operator", org, entitlement.Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", State: "active", Plan: "local-single", Hours: 24, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	op, err := store.Submit(ctx, owner, org, Request{Kind: "agent", Name: "real", Region: "local", Key: "request"})
	if err != nil {
		t.Fatal(err)
	}
	agent := &remoteAgent{ids: map[string]bool{}, failAfterCreate: true}
	provisioner := AgentProvisioner{Pool: pool, Client: agent}
	if err = provisioner.Step(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	if err = provisioner.Step(ctx, op.ID); err == nil {
		t.Fatal("uncertain response was accepted")
	}
	// Reconstruct after the host created its instance but the worker never checkpointed success.
	provisioner = AgentProvisioner{Pool: pool, Client: agent}
	if err = provisioner.Step(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	if err = provisioner.Step(ctx, op.ID); !errors.Is(err, ErrHealthPending) {
		t.Fatalf("premature readiness: %v", err)
	}
	agent.healthy = true
	if err = provisioner.Step(ctx, op.ID); err != nil {
		t.Fatal(err)
	}
	result, err := store.Get(ctx, owner, op.ID)
	if err != nil || result.State != "ready" || result.Kind != "agent" || len(result.Events) != 4 || len(agent.ids) != 1 {
		t.Fatalf("result: %+v instances %d %v", result, len(agent.ids), err)
	}
	if _, err = store.Submit(ctx, owner, org, Request{Name: "real", Region: "local", Key: "request"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("kind changed during replay: %v", err)
	}
	var placementCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements`).Scan(&placementCount); err != nil || placementCount != 1 {
		t.Fatalf("placements: %d %v", placementCount, err)
	}
	// An expired measurement cannot place a new instance.
	if _, err = pool.Exec(ctx, `UPDATE hosting_nodes SET last_verified_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	another, err := store.Submit(ctx, owner, org, Request{Name: "other", Region: "local", Key: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (fleet.Store{Pool: pool}).Reserve(ctx, another.ID); !errors.Is(err, fleet.ErrCapacity) {
		t.Fatalf("stale measurement: %v", err)
	}
}
