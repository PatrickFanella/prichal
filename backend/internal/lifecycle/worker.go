package lifecycle

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

type Agent interface {
	Service(context.Context, nodeagent.Peer, string, nodeagent.ServiceRequest) (nodeagent.ServiceResult, error)
}
type Worker struct {
	river.WorkerDefaults[Args]
	Pool  *pgxpool.Pool
	Agent Agent
}

func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	if err := recovery.Check(ctx, w.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	err := w.perform(ctx, job.Args.OperationID)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if job.Attempt >= job.MaxAttempts {
		tx, txErr := w.Pool.Begin(ctx)
		if txErr != nil {
			return errors.New("service result could not be recorded")
		}
		defer tx.Rollback(ctx)
		changed, txErr := tx.Exec(ctx, `UPDATE service_operations SET state='failed',updated_at=now() WHERE id=$1 AND state='pending'`, job.Args.OperationID)
		if txErr == nil && changed.RowsAffected() > 0 {
			_, txErr = tx.Exec(ctx, `INSERT INTO service_operation_events(operation_id,state) VALUES($1,'failed')`, job.Args.OperationID)
		}
		if txErr != nil {
			return errors.New("service result could not be recorded")
		}
		if txErr = tx.Commit(ctx); txErr != nil {
			return errors.New("service result could not be recorded")
		}
	}
	return errors.New("service transition could not be confirmed; retained resources require inspection")
}
func (w *Worker) NextRetry(job *river.Job[Args]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt) * time.Second)
}
func (w *Worker) perform(ctx context.Context, id string) error {
	if w.Agent == nil {
		return errors.New("authenticated lifecycle agent required")
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id::text FROM service_operations WHERE id=$1`, id).Scan(&deployment); err != nil {
		return err
	}
	var runtimeState string
	if err = tx.QueryRow(ctx, `SELECT state FROM deployments WHERE id=$1 AND kind='agent' FOR UPDATE`, deployment).Scan(&runtimeState); err != nil {
		return err
	}
	var state, desired string
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT state,desired_state,generation FROM service_operations WHERE id=$1 FOR UPDATE`, id).Scan(&state, &desired, &generation); err != nil {
		return err
	}
	if state != "pending" {
		return tx.Commit(ctx)
	}
	if runtimeState != "ready" {
		return ErrConflict
	}
	var latest int64
	if err = tx.QueryRow(ctx, `SELECT max(generation) FROM service_operations WHERE deployment_id=$1`, deployment).Scan(&latest); err != nil {
		return err
	}
	if latest != generation {
		return ErrConflict
	}
	var peer nodeagent.Peer
	if err = tx.QueryRow(ctx, `SELECT n.id::text,n.agent_endpoint,n.public_key_hash FROM instance_placements p JOIN hosting_nodes n ON n.id=p.node_id WHERE p.deployment_id=$1 AND n.state IN ('active','draining')`, deployment).Scan(&peer.NodeID, &peer.Endpoint, &peer.PublicKeyHash); err != nil {
		return err
	}
	request := nodeagent.ServiceRequest{Ticket: id, Generation: generation, State: desired}
	result, err := w.Agent.Service(ctx, peer, deployment, request)
	if err != nil || result.InstanceID != deployment || result.ServiceRequest != request {
		return errors.New("native service transition could not be confirmed")
	}
	if _, err = tx.Exec(ctx, `UPDATE service_operations SET state='applied',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO service_operation_events(operation_id,state) VALUES($1,'applied')`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
