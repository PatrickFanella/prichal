// Package recovery prevents a restored management database from executing uncertain jobs.
package recovery

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrHeld = errors.New("management recovery requires host reconciliation")

// Check fails closed when the ledger is unavailable or reconciliation remains pending.
func Check(ctx context.Context, pool *pgxpool.Pool) error {
	var held bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM management_recovery WHERE state='held')`).Scan(&held); err != nil {
		return errors.New("management recovery state unavailable")
	}
	if held {
		return ErrHeld
	}
	return nil
}

// Hold is restricted to disposable local restoration databases. It cannot release a hold.
// Operators must call it before starting any process connected to the restored database.
func Hold(ctx context.Context, pool *pgxpool.Pool, snapshot []byte) error {
	if len(snapshot) != 32 {
		return errors.New("snapshot digest required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var database string
	if err = tx.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		return err
	}
	if !strings.HasPrefix(database, "pds_recovery_") || len(database) < 20 {
		return errors.New("recovery hold requires a disposable local restore database")
	}
	_, err = tx.Exec(ctx, `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held') ON CONFLICT(singleton) DO UPDATE SET state='held',snapshot_sha256=excluded.snapshot_sha256,held_at=now(),reconciled_at=NULL`, snapshot)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM recovery_retained_namespaces`); err != nil {
		return err
	}
	// Restoring management state must not resurrect copied browser/CLI credentials
	// or unfinished authorization callbacks and device grants. New authentication follows reconciliation.
	if _, err = tx.Exec(ctx, `DELETE FROM management_sessions; DELETE FROM login_attempts; DELETE FROM cli_credentials; DELETE FROM cli_device_grants`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
