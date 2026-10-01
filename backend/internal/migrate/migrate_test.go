package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/app"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

// Integration tests use a separate disposable database created by scripts/smoke.sh.
func TestMigrationLifecycle(t *testing.T) {
	pool := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var err error
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

func TestVersionOneUpgradeAndUnownedData(t *testing.T) {
	for _, populated := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "unowned organization"}[populated], func(t *testing.T) {
			pool := testdb.New(t)
			ctx := context.Background()
			sql, err := migrations.ReadFile("sql/001_foundation.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, string(sql)+`; CREATE TABLE schema_version(singleton boolean PRIMARY KEY CHECK(singleton),version integer NOT NULL); INSERT INTO schema_version VALUES(true,1)`); err != nil {
				t.Fatal(err)
			}
			if populated {
				if _, err = pool.Exec(ctx, `INSERT INTO organizations(id,name) VALUES(gen_random_uuid(),'Existing unowned data')`); err != nil {
					t.Fatal(err)
				}
			}
			err = Apply(ctx, pool)
			if !populated {
				if err != nil {
					t.Fatal(err)
				}
				if err = (Probe{pool}).Check(ctx); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("upgrade assigned unowned data implicitly")
			}
			var version, count int
			if err = pool.QueryRow(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if version != 1 || count != 1 {
				t.Fatal("failed upgrade did not preserve schema version and data")
			}
		})
	}
}
