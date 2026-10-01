package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/app"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestManagementReadinessTracksRecoveryAfterStartup(t *testing.T) {
	pool := testdb.NewRecovery(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	handler := app.Handler(managementProbe{Pool: pool})
	check := func(want int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/readiness", nil))
		if response.Code != want {
			t.Fatalf("readiness got %d, want %d", response.Code, want)
		}
		if response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "snapshot") || strings.Contains(response.Body.String(), "reconciliation") {
			t.Fatal("readiness leaked recovery details or was cacheable")
		}
		expected := `"ready":false`
		if want == 200 {
			expected = `"ready":true`
		}
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatal("readiness authority inconsistent")
		}
	}
	check(200)
	if err := recovery.Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	check(503)
	if err := recovery.Reconcile(ctx, pool, emptyReadinessObserver{}); err != nil {
		t.Fatal(err)
	}
	check(200)
	if _, err := pool.Exec(ctx, `DROP TABLE management_recovery`); err != nil {
		t.Fatal(err)
	}
	check(503)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE schema_version SET version=999`); err != nil {
		t.Fatal(err)
	}
	check(503)
	pool.Close()
	check(503)
}

type emptyReadinessObserver struct{}

func (emptyReadinessObserver) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{}, errors.New("fixture has no hosts")
}
func (emptyReadinessObserver) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	return nodeagent.Instance{}, errors.New("fixture has no hosts")
}
