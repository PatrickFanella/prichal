// Package migrate manages the schema owned by the management service, never PDS data.
package migrate

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const Version = 3

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
		file := map[int]string{1: "sql/001_foundation.sql", 2: "sql/002_owner_auth.sql", 3: "sql/003_deployments.sql"}[next]
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
