package deployment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// TestProvisioner performs no host actions. Each step commits a recoverable checkpoint.
type TestProvisioner struct{ Pool *pgxpool.Pool }

func (p TestProvisioner) Step(ctx context.Context, id string) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM deployments WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
		return err
	}
	var next, message string
	switch state {
	case "queued":
		next, message = "configuring", "Configuring the local test instance."
	case "configuring":
		if _, err = tx.Exec(ctx, `INSERT INTO test_instances(deployment_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
			return err
		}
		next, message = "checking", "Checking the local test instance."
	case "checking":
		next, message = "ready", "Local test complete. No PDS has been deployed."
	case "ready", "failed":
		return tx.Commit(ctx)
	default:
		return errors.New("unsupported deployment checkpoint")
	}
	if _, err = tx.Exec(ctx, `UPDATE deployments SET state=$2,message=$3,updated_at=now() WHERE id=$1`, id, next, message); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,$2,$3)`, id, next, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Provisioner interface {
	Step(context.Context, string) error
}

type Worker struct {
	river.WorkerDefaults[Args]
	Provisioner Provisioner
	Pool        *pgxpool.Pool
}

func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	for i := 0; i < 3; i++ {
		if err := w.Provisioner.Step(ctx, job.Args.DeploymentID); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if job.Attempt >= job.MaxAttempts {
				tx, txErr := w.Pool.Begin(ctx)
				if txErr != nil {
					return errors.New("deployment could not be checkpointed")
				}
				defer tx.Rollback(ctx)
				result, txErr := tx.Exec(ctx, `UPDATE deployments SET state='failed',message='The local test could not finish. Contact support.',updated_at=now() WHERE id=$1 AND state NOT IN ('ready','failed')`, job.Args.DeploymentID)
				if txErr == nil && result.RowsAffected() > 0 {
					_, txErr = tx.Exec(ctx, `INSERT INTO deployment_events(deployment_id,state,message) VALUES($1,'failed','The local test could not finish. Contact support.')`, job.Args.DeploymentID)
				}
				if txErr != nil {
					return errors.New("deployment could not be checkpointed")
				}
				if txErr = tx.Commit(ctx); txErr != nil {
					return errors.New("deployment could not be checkpointed")
				}
			}
			return errors.New("local provisioning temporarily unavailable")
		}
	}
	return nil
}
func (w *Worker) NextRetry(job *river.Job[Args]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt) * time.Second)
}
