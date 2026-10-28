package main

import (
	"context"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type stagingAgent struct{}

func (stagingAgent) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{MaximumSlots: 2, CPUs: 8, MemoryBytes: 8 << 30}, nil
}
func (stagingAgent) Ensure(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: true}, nil
}

func TestStagingWorkerOnlyConsumesPrivateDeploymentQueue(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('qualification','owner','Owner') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Staging',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1)`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := (entitlement.Store{Pool: pool}).Submit(ctx, "qualification", org, entitlement.Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", State: "active", Plan: "local-small", Hours: 1, Reason: "qualification"}); err != nil {
		t.Fatal(err)
	}
	store, err := deployment.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	simulation, err := store.Submit(ctx, owner, org, deployment.Request{Kind: "test", Name: "simulation", Region: "local", Key: "simulation"})
	if err != nil {
		t.Fatal(err)
	}
	store.Staging = true
	native, err := store.Submit(ctx, owner, org, deployment.Request{Kind: "agent", Name: "native", Region: "local", Key: "native"})
	if err != nil {
		t.Fatal(err)
	}
	store.ManagedIntents = true
	managed, err := store.Submit(ctx, owner, org, deployment.Request{Kind: "agent", Name: "managed", Region: "local", Key: "managed"})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- runStagingWorker(workerCtx, pool, stagingAgent{}) }()
	defer func() {
		stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		op, err := store.Get(ctx, owner, native.ID)
		if err != nil {
			t.Fatal(err)
		}
		var snoozed bool
		if err := pool.QueryRow(ctx, `SELECT state='scheduled' AND scheduled_at>now()+interval '20 seconds' FROM river_job WHERE args->>'deploymentId'=$1`, managed.ID).Scan(&snoozed); err != nil {
			t.Fatal(err)
		}
		if op.State == "ready" && snoozed {
			break
		}
		select {
		case err := <-done:
			done <- err
			t.Fatalf("worker exited: %v", err)
		case <-deadline.C:
			t.Fatal("staging deployment did not complete")
		case <-tick.C:
		}
	}
	retained, err := store.Get(ctx, owner, simulation.ID)
	if err != nil || retained.State != "queued" {
		t.Fatalf("default simulation consumed: %+v %v", retained, err)
	}
	var instances int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM test_instances`).Scan(&instances); err != nil || instances != 0 {
		t.Fatalf("simulation runtime created: %d %v", instances, err)
	}
	retained, err = store.Get(ctx, owner, managed.ID)
	if err != nil || retained.State != "queued" || retained.Endpoint != "" || len(retained.Events) != 1 {
		t.Fatal("managed intent activated or lost its pending checkpoint", retained, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM instance_placements WHERE deployment_id=$1`, managed.ID).Scan(&instances); err != nil || instances != 0 {
		t.Fatal("managed intent reserved native placement", err)
	}
}
