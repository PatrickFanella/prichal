// Package testdb allocates isolated databases only on explicit local/CI connections.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func New(t *testing.T) *pgxpool.Pool { return create(t, "pds_smoke_", 8) }

// NewRecovery creates only a disposable recovery copy on the same guarded local/CI parent.
func NewRecovery(t *testing.T) *pgxpool.Pool { return create(t, "pds_recovery_", 16) }

func create(t *testing.T, prefix string, size int) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("PDS_TEST_ADMIN_URL")
	if url == "" {
		t.Skip("requires local/CI PDS_TEST_ADMIN_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var parent string
	if err = admin.QueryRow(ctx, `SELECT current_database()`).Scan(&parent); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	if parent != "pds_local" && parent != "pds_ci" {
		admin.Close()
		t.Fatal("test admin must target pds_local or pds_ci")
	}
	bytes := make([]byte, size)
	if _, err = rand.Read(bytes); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	name := prefix + hex.EncodeToString(bytes)
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		// Forced drops can wait on checkpoint I/O when many packages run in parallel.
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop disposable database: %v", err)
		}
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}
