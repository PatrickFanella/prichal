package main

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// replay is a qualification-only reconstruction of a lost management checkpoint.
// The executable has already restricted its connection to a disposable local copy.
func replay(ctx context.Context, pool *pgxpool.Pool, agent *nodeagent.Client, id string) error {
	if err := recovery.Check(ctx, pool); err != nil {
		return err
	}
	var peer nodeagent.Peer
	if err := pool.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM deployments d JOIN instance_placements p ON p.deployment_id=d.id JOIN hosting_nodes n ON n.id=p.node_id WHERE d.id=$1 AND d.kind='agent' AND d.state='ready'`, id).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return errors.New("ready placed local runtime required for replay")
	}
	instance, err := agent.Status(ctx, peer, id)
	if err != nil || !instance.Healthy {
		return errors.New("existing healthy runtime required before replay")
	}
	if err = deployment.Migrate(ctx, pool); err != nil {
		return err
	}
	const qualificationQueue = "recovery_qualification"
	workers := river.NewWorkers()
	river.AddWorker(workers, &deployment.Worker{Pool: pool, Provisioner: deployment.Router{Pool: pool, Agent: agent}})
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{qualificationQueue: {MaxWorkers: 1}}})
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE deployments SET state='checking',message='Replaying a lost checkpoint in the disposable recovery copy.',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'checking','Disposable recovery lost-checkpoint replay.')`, id); err != nil {
		return err
	}
	inserted, err := client.InsertTx(ctx, tx, deployment.Args{DeploymentID: id}, &river.InsertOpts{MaxAttempts: 3, Queue: qualificationQueue})
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if err = client.Start(ctx); err != nil {
		return err
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Stop(shutdown)
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state, jobState string
		if err = pool.QueryRow(ctx, `SELECT d.state,j.state::text FROM deployments d CROSS JOIN river_job j WHERE d.id=$1 AND j.id=$2`, id, inserted.Job.ID).Scan(&state, &jobState); err != nil {
			return err
		}
		if state == "ready" && jobState == "completed" {
			return nil
		}
		if state == "failed" || jobState == "discarded" || jobState == "cancelled" {
			return errors.New("recovery replay did not complete")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
