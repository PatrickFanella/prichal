package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Probe PostgreSQL from another connection immediately before DROP is issued.
// This makes admission at the teardown boundary observable without a timing race.
type reconnectAtDrop struct {
	config    *pgx.ConnConfig
	attempted bool
	code      string
	err       error
}

func (p *reconnectAtDrop) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "DROP DATABASE ") {
		p.attempted = true
		conn, err := pgx.ConnectConfig(ctx, p.config)
		if err == nil {
			p.err = conn.Close(ctx)
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				p.code = pgErr.Code
			} else {
				p.err = err
			}
		}
	}
	return ctx
}

func (*reconnectAtDrop) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestDropDisposableDatabaseFencesReconnects(t *testing.T) {
	url := os.Getenv("PDS_TEST_ADMIN_URL")
	if url == "" {
		t.Skip("requires local/CI PDS_TEST_ADMIN_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var parent string
	if err = admin.QueryRow(ctx, `SELECT current_database()`).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent != "pds_local" && parent != "pds_ci" {
		t.Fatal("test admin must target pds_local or pds_ci")
	}
	for _, prefix := range []string{"pds_smoke_", "pds_recovery_"} {
		t.Run(prefix, func(t *testing.T) {
			size := 8
			if prefix == "pds_recovery_" {
				size = 16
			}
			bytes := make([]byte, size)
			if _, err := rand.Read(bytes); err != nil {
				t.Fatal(err)
			}
			name := prefix + hex.EncodeToString(bytes)
			identifier := pgx.Identifier{name}.Sanitize()
			if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if _, err := admin.Exec(cleanup, "DROP DATABASE IF EXISTS "+identifier+" WITH (FORCE)"); err != nil {
					t.Error(err)
				}
			}()
			cfg := admin.Config().ConnConfig.Copy()
			cfg.Database = name
			existing, err := pgx.ConnectConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer existing.Close(context.Background())
			wrongParentConfig := admin.Config()
			wrongParentConfig.ConnConfig.Database = name
			wrongParent, err := pgxpool.NewWithConfig(ctx, wrongParentConfig)
			if err != nil {
				t.Fatal(err)
			}
			if err := dropDisposableDatabase(ctx, wrongParent, name); err == nil {
				t.Fatal("cleanup accepted a non-local/CI parent")
			}
			wrongParent.Close()
			var acceptsConnections bool
			if err := admin.QueryRow(ctx, `SELECT datallowconn FROM pg_database WHERE datname=$1`, name).Scan(&acceptsConnections); err != nil || !acceptsConnections {
				t.Fatal("refused cleanup changed database admission", err)
			}
			probe := &reconnectAtDrop{config: cfg}
			tracedConfig := admin.Config()
			tracedConfig.ConnConfig.Tracer = probe
			traced, err := pgxpool.NewWithConfig(ctx, tracedConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer traced.Close()
			if err := dropDisposableDatabase(ctx, traced, name); err != nil {
				t.Fatal(err)
			}
			if !probe.attempted || probe.err != nil || probe.code != "55000" {
				t.Fatalf("cleanup admitted a new connection before drop: attempted=%v code=%q err=%v", probe.attempted, probe.code, probe.err)
			}
			if err := existing.Ping(ctx); err == nil {
				t.Fatal("cleanup retained an existing session")
			}
			var remains bool
			if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&remains); err != nil || remains {
				t.Fatal("disposable database remains after cleanup", err)
			}
		})
	}
}

func TestDropDisposableDatabaseRejectsOtherNames(t *testing.T) {
	for _, name := range []string{"pds_local", "pds_ci", "postgres", "production", "pds_smoke_", "pds_smoke_0123456789abcdeG", "pds_smoke_0123456789abcdef; DROP DATABASE postgres", "pds_recovery_0123456789abcdef"} {
		if err := dropDisposableDatabase(context.Background(), nil, name); err == nil {
			t.Fatalf("accepted unsafe database name %q", name)
		}
	}
}
