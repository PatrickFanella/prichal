package observability

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type fixture struct{ down bool }

func (f fixture) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	if f.down {
		return nodeagent.Capacity{}, errors.New("PRIVATE SECRET MUST NEVER ENTER OBSERVATIONS")
	}
	return nodeagent.Capacity{MaximumSlots: 0}, nil
}
func (f fixture) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	return nodeagent.Instance{}, errors.New("PRIVATE RECORD")
}
func TestScanDeduplicatesAndResolvesWithoutRecordingHostErrors(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',1,1,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := Scan(ctx, pool, fixture{down: true}); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM service_observations WHERE subject_key=$1 AND kind='host_unavailable' AND active`, "node/"+id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("deduplication: %d %v", count, err)
	}
	if _, err := Scan(ctx, pool, fixture{}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM service_observations WHERE subject_key=$1 AND kind='host_unavailable' AND NOT active AND resolved_at IS NOT NULL`, "node/"+id).Scan(&count); err != nil || count != 1 {
		t.Fatal("recovery not recorded")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM service_observations WHERE kind='capacity_exhausted' AND active`).Scan(&count); err != nil || count != 1 {
		t.Fatal("capacity exhaustion not recorded")
	}
	if err := Record(ctx, pool, []Finding{{Subject: "invalid", Kind: "SECRET"}}); err == nil {
		t.Fatal("unbounded finding accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM service_observations WHERE subject_key='invalid'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed scan was not atomic")
	}
}

func TestOwnerStatusNeverTreatsOldObservationAsCurrent(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','monitor','Monitor') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Monitor',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,'monitor','local','monitor','test','ready','monitor.localhost') RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	status, err := DeploymentStatus(ctx, pool, id)
	if err != nil || status.State != "unknown" || status.CheckedAt != nil || status.Backup != "unconfigured" {
		t.Fatalf("unknown status: %+v %v", status, err)
	}
	if err = Record(ctx, pool, []Finding{{Subject: "deployment/" + id, DeploymentID: id, Kind: "runtime_unhealthy", Active: false}}); err != nil {
		t.Fatal(err)
	}
	status, err = DeploymentStatus(ctx, pool, id)
	if err != nil || status.State != "healthy" {
		t.Fatal("current observation missing")
	}
	if _, err = pool.Exec(ctx, `UPDATE service_observations SET last_observed_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	status, err = DeploymentStatus(ctx, pool, id)
	if err != nil || status.State != "stale" {
		t.Fatal("stale observation claimed current")
	}
}
