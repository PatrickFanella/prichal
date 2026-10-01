// Package migrate manages the schema owned by the management service, never PDS data.
package migrate

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const Version = 22

//go:embed sql/*.sql
var migrations embed.FS

// Apply serializes startup migrations and commits the migration and version together.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin management migration: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(73647301)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_version (singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), version integer NOT NULL CHECK(version >= 0)); INSERT INTO schema_version (singleton,version) VALUES(true,0) ON CONFLICT DO NOTHING`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRow(ctx, `SELECT version FROM schema_version WHERE singleton=true`).Scan(&version); err != nil {
		return err
	}
	if version > Version {
		return fmt.Errorf("incompatible management schema: database version %d exceeds supported version %d", version, Version)
	}
	for next := version + 1; next <= Version; next++ {
		file := map[int]string{1: "sql/001_foundation.sql", 2: "sql/002_owner_auth.sql", 3: "sql/003_deployments.sql", 4: "sql/004_fleet.sql", 5: "sql/005_runtime_operations.sql", 6: "sql/006_managed_endpoints.sql", 7: "sql/007_local_tls.sql", 8: "sql/008_recovery_hold.sql", 9: "sql/009_retained_recovery_namespaces.sql", 10: "sql/010_service_observations.sql", 11: "sql/011_owner_enrollment.sql", 12: "sql/012_handle_proof.sql", 13: "sql/013_node_capacity_policy.sql", 14: "sql/014_native_usage.sql", 15: "sql/015_service_lifecycle.sql", 16: "sql/016_account_lifecycle.sql", 17: "sql/017_cli_authorization.sql", 18: "sql/018_pilot_entitlements.sql", 19: "sql/019_billing_fixture_inbox.sql", 20: "sql/020_support_inspections.sql", 21: "sql/021_customer_exports.sql", 22: "sql/022_hosting_exit.sql"}[next]
		sql, err := migrations.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply management migration %d: %w", next, err)
		}
		if _, err = tx.Exec(ctx, `UPDATE schema_version SET version=$1 WHERE singleton=true`, next); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

type Probe struct{ Pool *pgxpool.Pool }

func (p Probe) Check(ctx context.Context) error {
	var version int
	if err := p.Pool.QueryRow(ctx, `SELECT version FROM schema_version WHERE singleton=true`).Scan(&version); err != nil {
		return err
	}
	if version != Version {
		return fmt.Errorf("management schema must be version %d", Version)
	}
	return nil
}

func (p Probe) Version() int { return Version }
