package hostingexit

import (
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/riverqueue/river"
	"time"
)

type RetentionArgs struct {
	ExitID string `json:"exitId"`
}

func (RetentionArgs) Kind() string { return "hosting_exit_retention" }

// RetentionWorker records review due. It has no host execution or deletion capability.
type RetentionWorker struct {
	river.WorkerDefaults[RetentionArgs]
	Store *Store
}

func (w *RetentionWorker) Work(ctx context.Context, job *river.Job[RetentionArgs]) error {
	if err := recovery.Check(ctx, w.Store.Pool); err != nil {
		return river.JobSnooze(time.Minute)
	}
	tx, err := w.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var deployment string
	if err = tx.QueryRow(ctx, `SELECT deployment_id FROM hosting_exits WHERE id=$1`, job.Args.ExitID).Scan(&deployment); err != nil {
		return errors.New("retained exit reference unavailable")
	}
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM deployments WHERE id=$1 FOR UPDATE`, deployment).Scan(&found); err != nil {
		return errors.New("retained exit deployment unavailable")
	}
	var state string
	var until float64
	if err = tx.QueryRow(ctx, `SELECT state,greatest(0,extract(epoch FROM retention_review_at-clock_timestamp()))::double precision FROM hosting_exits WHERE id=$1 FOR UPDATE`, job.Args.ExitID).Scan(&state, &until); err != nil {
		return errors.New("retention review unavailable")
	}
	if state == "reversed" || state == "review" {
		return tx.Commit(ctx)
	}
	if until > 0 {
		return river.JobSnooze(time.Duration(until * float64(time.Second)))
	}
	if _, err = tx.Exec(ctx, `UPDATE hosting_exits SET state='review',updated_at=now() WHERE id=$1`, job.Args.ExitID); err != nil {
		return errors.New("retention review unavailable")
	}
	if err = event(ctx, tx, job.Args.ExitID, "review"); err != nil {
		return errors.New("retention audit unavailable")
	}
	return tx.Commit(ctx)
}
