package enrollment

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

// ErrAdmissionClosed prevents new intent without invalidating retained requests.
var ErrAdmissionClosed = errors.New("new enrollment admission is closed")

// Admission is management policy, not permission to activate production serving.
type Admission struct {
	Open      bool      `json:"open"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Admission returns the current durable first-account request policy without operator identity.
func (s *Store) Admission(ctx context.Context) (Admission, error) {
	var result Admission
	err := s.Pool.QueryRow(ctx, `SELECT open,updated_at FROM enrollment_admission WHERE singleton=true`).Scan(&result.Open, &result.UpdatedAt)
	return result, err
}

// OwnedAdmission returns policy only after checking current deployment ownership.
func (s *Store) OwnedAdmission(ctx context.Context, owner, deployment string) (Admission, error) {
	if !uuid.MatchString(owner) || !uuid.MatchString(deployment) {
		return Admission{}, ErrNotFound
	}
	var found string
	err := s.Pool.QueryRow(ctx, `SELECT d.id FROM deployments d JOIN organizations o ON o.id=d.organization_id WHERE d.id=$1 AND o.owner_id=$2`, deployment, owner).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admission{}, ErrNotFound
	}
	if err != nil {
		return Admission{}, err
	}
	return s.Admission(ctx)
}

// SetAdmission records an effective operator change atomically with its audit.
// Closing waits for transactions already admitted to commit or roll back.
func (s *Store) SetAdmission(ctx context.Context, actor, reason string, open bool) (Admission, error) {
	if len(actor) < 1 || len(actor) > 128 || strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return Admission{}, ErrInvalid
	}
	switch reason {
	case "maintenance", "safety_review", "capacity_review", "local_qualification":
	default:
		return Admission{}, ErrInvalid
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return Admission{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Admission{}, err
	}
	defer tx.Rollback(ctx)
	var result Admission
	if err = tx.QueryRow(ctx, `SELECT open,updated_at FROM enrollment_admission WHERE singleton=true FOR UPDATE`).Scan(&result.Open, &result.UpdatedAt); err != nil {
		return Admission{}, err
	}
	if result.Open != open {
		if _, err = tx.Exec(ctx, `INSERT INTO enrollment_admission_events(actor,reason,previous_open,requested_open) VALUES($1,$2,$3,$4)`, actor, reason, result.Open, open); err != nil {
			return Admission{}, err
		}
		if err = tx.QueryRow(ctx, `UPDATE enrollment_admission SET open=$1,updated_at=clock_timestamp() WHERE singleton=true RETURNING open,updated_at`, open).Scan(&result.Open, &result.UpdatedAt); err != nil {
			return Admission{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Admission{}, err
	}
	return result, nil
}

// checkAdmission holds a shared policy lock through intent/job commit. Missing
// policy fails closed. Stable request replay takes place before this check.
func checkAdmission(ctx context.Context, tx pgx.Tx) error {
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return err
	}
	if held {
		return recovery.ErrHeld
	}
	var open bool
	if err := tx.QueryRow(ctx, `SELECT open FROM enrollment_admission WHERE singleton=true FOR SHARE`).Scan(&open); err != nil {
		return err
	}
	if !open {
		return ErrAdmissionClosed
	}
	return nil
}
