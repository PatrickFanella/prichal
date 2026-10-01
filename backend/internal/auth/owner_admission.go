package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOwnerAdmissionClosed blocks new management owners, not existing owner sign-in.
var ErrOwnerAdmissionClosed = errors.New("new management owners are paused")

// OwnerAdmission describes management-owner creation policy, not native hosting access.
type OwnerAdmission struct {
	Open      bool      `json:"open"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// OwnerAdmissionStore has no issuer, native credentials or hosting access.
type OwnerAdmissionStore struct{ Pool *pgxpool.Pool }

// Status returns durable policy without operator identity.
func (s OwnerAdmissionStore) Status(ctx context.Context) (OwnerAdmission, error) {
	var result OwnerAdmission
	err := s.Pool.QueryRow(ctx, `SELECT open,updated_at FROM owner_admission WHERE singleton=true`).Scan(&result.Open, &result.UpdatedAt)
	return result, err
}

// Set records effective changes atomically and waits for already-admitted owner creation.
func (s OwnerAdmissionStore) Set(ctx context.Context, actor, reason string, open bool) (OwnerAdmission, error) {
	if len(actor) < 1 || len(actor) > 128 || strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return OwnerAdmission{}, errors.New("invalid owner admission actor")
	}
	switch reason {
	case "maintenance", "safety_review", "capacity_review", "local_qualification":
	default:
		return OwnerAdmission{}, errors.New("invalid owner admission reason")
	}
	if err := recovery.Check(ctx, s.Pool); err != nil {
		return OwnerAdmission{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return OwnerAdmission{}, err
	}
	defer tx.Rollback(ctx)
	var result OwnerAdmission
	if err = tx.QueryRow(ctx, `SELECT open,updated_at FROM owner_admission WHERE singleton=true FOR UPDATE`).Scan(&result.Open, &result.UpdatedAt); err != nil {
		return OwnerAdmission{}, err
	}
	if result.Open != open {
		if _, err = tx.Exec(ctx, `INSERT INTO owner_admission_events(actor,reason,previous_open,requested_open) VALUES($1,$2,$3,$4)`, actor, reason, result.Open, open); err != nil {
			return OwnerAdmission{}, err
		}
		if err = tx.QueryRow(ctx, `UPDATE owner_admission SET open=$1,updated_at=clock_timestamp() WHERE singleton=true RETURNING open,updated_at`, open).Scan(&result.Open, &result.UpdatedAt); err != nil {
			return OwnerAdmission{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return OwnerAdmission{}, err
	}
	return result, nil
}

// ownerForSession holds admission through owner/session commit only for a new subject.
func ownerForSession(ctx context.Context, tx pgx.Tx, issuer, subject, name string) (string, error) {
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return "", err
	}
	if held {
		return "", recovery.ErrHeld
	}
	var id string
	// Existing owners can sign in while new creation is paused, without acquiring
	// the policy lock. The verified issuer and subject remain the identity key.
	err := tx.QueryRow(ctx, `UPDATE owners SET display_name=$3 WHERE issuer=$1 AND subject=$2 RETURNING id`, issuer, subject, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var open bool
	if err = tx.QueryRow(ctx, `SELECT open FROM owner_admission WHERE singleton=true FOR SHARE`).Scan(&open); err != nil {
		return "", err
	}
	if !open {
		// Another admitted callback may have committed this subject while closure
		// waited for its shared lock. It is now an existing owner, not a new signup.
		err = tx.QueryRow(ctx, `UPDATE owners SET display_name=$3 WHERE issuer=$1 AND subject=$2 RETURNING id`, issuer, subject, name).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		return "", ErrOwnerAdmissionClosed
	}
	err = tx.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES($1,$2,$3) ON CONFLICT(issuer,subject) DO UPDATE SET display_name=EXCLUDED.display_name RETURNING id`, issuer, subject, name).Scan(&id)
	return id, err
}
