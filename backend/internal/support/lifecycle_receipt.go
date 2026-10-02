package support

import (
	"context"
	"errors"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

type LifecycleReceipt struct {
	lifecycle.Operation
	IdempotencyKey string `json:"idempotencyKey"`
}

// LifecycleRequest inspects only this currently authorized operator's exact
// retained request. Inspection must commit before the safe receipt is returned.
func (s *Store) LifecycleRequest(ctx context.Context, owner, role, id, key string) (LifecycleReceipt, error) {
	if !uuid.MatchString(owner) || !uuid.MatchString(id) || !uuid.MatchString(key) || (role != "inspect" && role != "operate") {
		return LifecycleReceipt{}, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return LifecycleReceipt{}, err
	}
	defer tx.Rollback(ctx)
	var issuer, subject string
	err = tx.QueryRow(ctx, `SELECT issuer,subject FROM owners WHERE id=$1 FOR SHARE`, owner).Scan(&issuer, &subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return LifecycleReceipt{}, ErrNotFound
	}
	if err != nil {
		return LifecycleReceipt{}, err
	}
	currentRole := ""
	for _, identity := range s.Settings.Identities {
		if identity.Issuer == issuer && identity.Subject == subject {
			currentRole = identity.Role
			break
		}
	}
	if currentRole != role {
		return LifecycleReceipt{}, ErrNotFound
	}
	checkRecovery := func() error {
		var held bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
			return err
		}
		if held {
			return recovery.ErrHeld
		}
		return nil
	}
	if err := checkRecovery(); err != nil {
		return LifecycleReceipt{}, err
	}
	var receipt LifecycleReceipt
	var org string
	err = tx.QueryRow(ctx, `SELECT l.id::text,l.deployment_id::text,l.generation,l.desired_state,l.prior_state,l.reason,l.state,l.updated_at,l.kind,l.idempotency_key::text,d.organization_id::text FROM service_operations l JOIN deployments d ON d.id=l.deployment_id WHERE d.id=$1 AND l.idempotency_key=$2 AND l.actor=$3 FOR SHARE OF d,l`, id, key, "oidc-owner:"+owner).Scan(&receipt.ID, &receipt.DeploymentID, &receipt.Generation, &receipt.DesiredState, &receipt.PriorState, &receipt.Reason, &receipt.State, &receipt.UpdatedAt, &receipt.Kind, &receipt.IdempotencyKey, &org)
	if errors.Is(err, pgx.ErrNoRows) {
		return LifecycleReceipt{}, ErrNotFound
	}
	if err != nil {
		return LifecycleReceipt{}, err
	}
	if err := checkRecovery(); err != nil {
		return LifecycleReceipt{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO support_inspections(actor_owner_id,organization_id,deployment_id,role) VALUES($1,$2,$3,$4)`, owner, org, id, role); err != nil {
		return LifecycleReceipt{}, err
	}
	if err := checkRecovery(); err != nil {
		return LifecycleReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LifecycleReceipt{}, err
	}
	return receipt, nil
}
