package enrollment

import (
	"context"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
)

// Cancel only withdraws a queued request that has never reached native dispatch.
// Its original job is retained and becomes a no-op, including after recovery.
func (s *Store) Cancel(ctx context.Context, owner, id string) (Operation, error) {
	if !uuid.MatchString(owner) || !uuid.MatchString(id) {
		return Operation{}, ErrNotFound
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return Operation{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = lockOwnedEnrollment(ctx, tx, owner, id); err != nil {
		return Operation{}, err
	}
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return Operation{}, err
	}
	var state string
	var started *time.Time
	if err = tx.QueryRow(ctx, `SELECT state,dispatch_started_at FROM enrollment_operations WHERE id=$1 FOR UPDATE`, id).Scan(&state, &started); err != nil {
		return Operation{}, err
	}
	if state != "cancelled" {
		if state != "pending" || started != nil {
			return Operation{}, ErrConflict
		}
		const message = "The queued enrollment request was cancelled before native preparation started. No invitation was issued by this request."
		if _, err = tx.Exec(ctx, `UPDATE enrollment_operations SET state='cancelled',message=$2,updated_at=now() WHERE id=$1`, id, message); err != nil {
			return Operation{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO enrollment_events(enrollment_id,state,message) VALUES($1,'cancelled',$2)`, id, message); err != nil {
			return Operation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}

// Commit the checkpoint before any RPC. A failed/lost native result must never
// turn previously dispatched work back into an apparently cancellable request.
func (w *Worker) reserveDispatch(ctx context.Context, id string) (bool, error) {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var owner string
	if err = tx.QueryRow(ctx, `SELECT owner_id::text FROM enrollment_operations WHERE id=$1`, id).Scan(&owner); err != nil {
		return false, err
	}
	if _, err = lockOwnedEnrollment(ctx, tx, owner, id); err != nil {
		return false, err
	}
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return false, err
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM enrollment_operations WHERE id=$1 FOR UPDATE`, id).Scan(&state); err != nil {
		return false, err
	}
	if state == "pending" {
		if _, err = tx.Exec(ctx, `UPDATE enrollment_operations SET dispatch_started_at=coalesce(dispatch_started_at,clock_timestamp()) WHERE id=$1`, id); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return state == "cancelled", nil
}
