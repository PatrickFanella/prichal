package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/app"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration tests use a separate disposable database created by scripts/smoke.sh.
func TestMigrationLifecycle(t *testing.T) {
	url := os.Getenv("PDS_TEST_ADMIN_URL")
	if url == "" {
		t.Skip("requires local or CI PDS_TEST_ADMIN_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
		t.Fatal("smoke admin must connect to pds_local or pds_ci")
	}
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "pds_smoke_" + hex.EncodeToString(suffix[:])
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop smoke database: %v", err)
		}
	}()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = Apply(ctx, pool); err != nil {
		t.Fatalf("second startup: %v", err)
	}
	if err = (Probe{pool}).Check(ctx); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	app.Handler(Probe{pool}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/readiness", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ready":true`) {
		t.Fatalf("real database readiness: %d %s", w.Code, w.Body)
	}
	if _, err = pool.Exec(ctx, `UPDATE schema_version SET version=999`); err != nil {
		t.Fatal(err)
	}
	if err = Apply(ctx, pool); err == nil || !strings.Contains(err.Error(), "incompatible management schema") {
		t.Fatalf("future schema: %v", err)
	}
	if err = (Probe{pool}).Check(ctx); err == nil {
		t.Fatal("readiness accepted incompatible schema")
	}
	pool.Close()
	w = httptest.NewRecorder()
	app.Handler(Probe{pool}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/readiness", nil))
	if w.Code != 503 {
		t.Fatalf("closed database readiness: %d", w.Code)
	}
}
