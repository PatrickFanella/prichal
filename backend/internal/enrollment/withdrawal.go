package enrollment

import (
	"context"
	"errors"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// Withdraw identifies intent by the retained enrollment UUID. Repeated requests
// cannot add another job while the same withdrawal remains in flight.
func (s *Store) Withdraw(ctx context.Context, owner, id string) (Operation, error) {
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
	var state string
	var attempts int
	if err = tx.QueryRow(ctx, `SELECT state,withdrawal_attempts FROM enrollment_operations WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&state, &attempts); err != nil {
		return Operation{}, err
	}
	switch state {
	case "withdrawing", "withdrawn", "account_created", "verified":
		if err = tx.Commit(ctx); err != nil {
			return Operation{}, err
		}
		return s.Get(ctx, owner, id)
	case "invitation_ready", "expired", "failed":
	default:
		return Operation{}, ErrConflict
	}
	if attempts >= 3 {
		return Operation{}, ErrConflict
	}
	// Recheck the recovery ledger in this transaction before writing intent.
	if err = checkEnrollmentRecovery(ctx, tx); err != nil {
		return Operation{}, err
	}
	const message = "Invitation withdrawal requested. Native completion has not been confirmed. An account already created will be preserved."
	if _, err = tx.Exec(ctx, `UPDATE enrollment_operations SET state='withdrawing',code_ciphertext=NULL,withdrawal_requested_at=coalesce(withdrawal_requested_at,clock_timestamp()),withdrawal_attempts=withdrawal_attempts+1,message=$2,updated_at=now() WHERE id=$1`, id, message); err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO enrollment_events(enrollment_id,state,message) VALUES($1,'withdrawing',$2)`, id, message); err != nil {
		return Operation{}, err
	}
	if _, err = s.Queue.InsertTx(ctx, tx, Args{id}, &river.InsertOpts{MaxAttempts: 3}); err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return s.Get(ctx, owner, id)
}

// All mutation paths lock deployment before enrollment, matching the worker.
func lockOwnedEnrollment(ctx context.Context, tx pgx.Tx, owner, id string) (string, error) {
	var deployment string
	err := tx.QueryRow(ctx, `SELECT deployment_id::text FROM enrollment_operations WHERE id=$1 AND owner_id=$2`, id, owner).Scan(&deployment)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var state, kind string
	err = tx.QueryRow(ctx, `SELECT d.state,d.kind FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2 FOR UPDATE OF d FOR SHARE OF o`, deployment, owner).Scan(&state, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if state != "ready" || kind != "agent" {
		return "", ErrConflict
	}
	return deployment, nil
}

func checkEnrollmentRecovery(ctx context.Context, tx pgx.Tx) error {
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return err
	}
	if held {
		return recovery.ErrHeld
	}
	return nil
}
