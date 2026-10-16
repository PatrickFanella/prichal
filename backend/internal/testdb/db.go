// Package testdb allocates isolated databases only on explicit local/CI connections.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		if err := dropDisposableDatabase(cleanup, admin, name); err != nil {
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

var disposableName = regexp.MustCompile(`^(pds_smoke_[0-9a-f]{16}|pds_recovery_[0-9a-f]{32})$`)

func dropDisposableDatabase(ctx context.Context, admin *pgxpool.Pool, name string) error {
	if !disposableName.MatchString(name) {
		return errors.New("refusing cleanup of a non-disposable database name")
	}
	var parent string
	if err := admin.QueryRow(ctx, `SELECT current_database()`).Scan(&parent); err != nil {
		return err
	}
	if parent != "pds_local" && parent != "pds_ci" {
		return errors.New("test admin must target pds_local or pds_ci")
	}
	identifier := pgx.Identifier{name}.Sanitize()
	// FORCE terminates existing sessions but does not fence new connection attempts.
	// Close admission first so an asynchronous reconnect cannot race the drop.
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+identifier+" ALLOW_CONNECTIONS false"); err != nil {
		return err
	}
	_, err := admin.Exec(ctx, "DROP DATABASE "+identifier+" WITH (FORCE)")
	return err
}
