package billing

import (
	"context"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"github.com/jackc/pgx/v5"
)

// billingRecovery fails closed after transaction waits and before committed authority.
func billingRecovery(ctx context.Context, tx pgx.Tx) error {
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return ErrUnavailable
	}
	if held {
		return recovery.ErrHeld
	}
	return nil
}
