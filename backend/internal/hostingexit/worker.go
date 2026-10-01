package hostingexit

import (
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/riverqueue/river"
	"time"
)

type Worker struct {
	river.WorkerDefaults[Args]
	Store *Store
}

func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	if err := recovery.Check(ctx, w.Store.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	err := w.advance(ctx, job.Args.ExitID)
	var snooze *river.JobSnoozeError
	if err == nil || errors.As(err, &snooze) || ctx.Err() != nil || job.Attempt < job.MaxAttempts {
		return err
	}
	if reviewErr := w.review(ctx, job.Args.ExitID); reviewErr != nil {
		return errors.New("exit outcome requires review; retained data is unchanged")
	}
	return errors.New("exit outcome requires review; retained data is unchanged")
}
func (w *Worker) NextRetry(job *river.Job[Args]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt) * time.Second)
}
func (w *Worker) advance(ctx context.Context, id string) error {
	tx, err := w.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment, owner string
	if err = tx.QueryRow(ctx, `SELECT deployment_id,owner_id FROM hosting_exits WHERE id=$1`, id).Scan(&deployment, &owner); err != nil {
		return errors.New("retained exit reference unavailable")
	}
	var currentOwner string
	if err = tx.QueryRow(ctx, `SELECT o.owner_id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 FOR UPDATE OF d`, deployment).Scan(&currentOwner); err != nil {
		return errors.New("exit ownership unavailable")
	}
	var state string
	var until float64
	var stop, resume *string
	if err = tx.QueryRow(ctx, `SELECT state,greatest(0,extract(epoch FROM service_end_at-clock_timestamp()))::double precision,stop_operation_id,resume_operation_id FROM hosting_exits WHERE id=$1 FOR UPDATE`, id).Scan(&state, &until, &stop, &resume); err != nil {
		return errors.New("retained exit state unavailable")
	}
	if state == "reversed" || state == "ended" || state == "review" {
		return tx.Commit(ctx)
	}
	next := "review"
	snooze := false
	if currentOwner == owner {
		switch state {
		case "scheduled":
			if until > 0 {
				return river.JobSnooze(time.Duration(until * float64(time.Second)))
			}
			stopID, transitionErr := w.Store.Lifecycle.InsertExitTx(ctx, tx, id, deployment, id, "")
			if transitionErr == nil {
				if _, err = tx.Exec(ctx, `UPDATE hosting_exits SET stop_operation_id=$2 WHERE id=$1`, id, stopID); err != nil {
					return err
				}
				next = "stopping"
				snooze = true
			} else if !errors.Is(transitionErr, lifecycle.ErrConflict) {
				return errors.New("exit stop intent unavailable")
			}
		case "stopping", "resuming":
			operation := stop
			if state == "resuming" {
				operation = resume
			}
			if operation == nil {
				return errors.New("exit lifecycle reference unavailable")
			}
			var phase string
			if err = tx.QueryRow(ctx, `SELECT state FROM service_operations WHERE id=$1`, *operation).Scan(&phase); err != nil {
				return errors.New("exit lifecycle outcome unavailable")
			}
			if phase == "pending" {
				return river.JobSnooze(2 * time.Second)
			}
			if phase == "applied" {
				next = "ended"
				if state == "resuming" {
					next = "reversed"
				}
			}
		default:
			return errors.New("unsupported retained exit state")
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE hosting_exits SET state=$2,updated_at=now() WHERE id=$1`, id, next); err != nil {
		return errors.New("exit checkpoint unavailable")
	}
	if err = event(ctx, tx, id, next); err != nil {
		return errors.New("exit audit unavailable")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("exit checkpoint unavailable")
	}
	if snooze {
		return river.JobSnooze(2 * time.Second)
	}
	return nil
}

func (w *Worker) review(ctx context.Context, id string) error {
	tx, err := w.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id FROM hosting_exits WHERE id=$1`, id).Scan(&deployment); err != nil {
		return err
	}
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM deployments WHERE id=$1 FOR UPDATE`, deployment).Scan(&found); err != nil {
		return err
	}
	changed, err := tx.Exec(ctx, `UPDATE hosting_exits SET state='review',updated_at=now() WHERE id=$1 AND state NOT IN ('ended','reversed','review')`, id)
	if err != nil {
		return err
	}
	if changed.RowsAffected() > 0 {
		if err = event(ctx, tx, id, "review"); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
