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

func TestLocalTLSDataUpgrade(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('fixture','tls','TLS') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'TLS',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,'tls','local','tls-1','agent','ready','fixture.localhost','http://fixture.localhost:44292') RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// Version 7 changes data only. Reconstruct the preceding version's URL and marker.
	if _, err := pool.Exec(ctx, `DROP TABLE organization_access_events,organization_transfers,organization_invitations,organization_members; DROP TABLE hosting_exit_reversals,hosting_exit_events,hosting_exits; DROP TABLE export_events,export_operations; DROP TABLE support_inspections; DROP TABLE billing_subscription_observations,billing_events,billing_customer_bindings; DROP TABLE pilot_entitlement_operations; DROP TABLE cli_approval_attempts,cli_credentials,cli_device_grants; DROP TABLE service_operation_events,service_operations; DROP TABLE instance_usage; DROP TABLE hosting_node_policy_events; DROP TABLE handle_events,handle_configurations; DROP TABLE enrollment_events; DROP TABLE enrollment_operations; DROP TABLE service_probe_runs; DROP TABLE service_observations; DROP TABLE recovery_retained_namespaces; DROP TABLE management_recovery; UPDATE schema_version SET version=6`); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var hostname, endpoint, state string
	if err := pool.QueryRow(ctx, `SELECT managed_hostname,endpoint_url,state FROM deployments WHERE id=$1`, id).Scan(&hostname, &endpoint, &state); err != nil {
		t.Fatal(err)
	}
	if hostname != "fixture.localhost" || endpoint != "https://fixture.localhost:44292" || state != "ready" {
		t.Fatalf("unexpected identity change: %s %s %s", hostname, endpoint, state)
	}
	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM deployment_events WHERE deployment_id=$1`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("upgrade audit: %d %v", events, err)
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
