package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type fixture struct{ down bool }

func (f fixture) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	if f.down {
		return nodeagent.Capacity{}, errors.New("PRIVATE SECRET MUST NEVER ENTER OBSERVATIONS")
	}
	return nodeagent.Capacity{MaximumSlots: 0, CPUs: 3, MemoryBytes: 4 << 30}, nil
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

func (f fixture) Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error) {
	return nodeagent.Usage{}, errors.New("PRIVATE USAGE")
}

func TestUsageRetainsPriorSampleAndRejectsOlderReplacement(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','usage','Usage') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Usage',$1) RETURNING id::text`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,'usage','local','usage','agent','ready','usage.localhost') RETURNING id::text`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	unknown, err := DeploymentUsage(ctx, pool, id)
	if err != nil || unknown.State != "unknown" || unknown.StorageLimit != "unconfigured" {
		t.Fatal("unmeasured usage claimed known")
	}
	sample := nodeagent.Usage{InstanceID: id, DataBytes: 1000, BlobBytes: 68, DataFiles: 20, BlobFiles: 1, Accounts: 1, FilesystemBytes: 8 << 30, FilesystemAvailableBytes: 4 << 30, ObservedAtMillis: time.Now().UnixMilli()}
	if err = recordScan(ctx, pool, nil, []nodeagent.Usage{sample}); err != nil {
		t.Fatal(err)
	}
	observed, err := DeploymentUsage(ctx, pool, id)
	if err != nil || observed.State != "observed" || observed.DataBytes != 1000 || observed.BlobBytes != 68 {
		t.Fatal("usage sample lost")
	}
	if err = Record(ctx, pool, []Finding{{Subject: "deployment/" + id, DeploymentID: id, Kind: "usage_unavailable", Active: true}}); err != nil {
		t.Fatal(err)
	}
	retained, err := DeploymentUsage(ctx, pool, id)
	if err != nil || retained.DataBytes != 1000 || retained.State != "unavailable" {
		t.Fatal("failed probe zeroed retained usage")
	}
	sample.DataBytes = 1
	sample.ObservedAtMillis -= 1000
	if err = recordScan(ctx, pool, nil, []nodeagent.Usage{sample}); err != nil {
		t.Fatal(err)
	}
	retained, err = DeploymentUsage(ctx, pool, id)
	if err != nil || retained.DataBytes != 1000 {
		t.Fatal("old scan replaced newer measurement")
	}
	if _, err = pool.Exec(ctx, `UPDATE instance_usage SET measured_at=now()-interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	stale, err := DeploymentUsage(ctx, pool, id)
	if err != nil || stale.State != "stale" || stale.DataBytes != 1000 {
		t.Fatal("stale usage discarded or claimed current")
	}
	sample.DataBytes = -1
	if err = recordScan(ctx, pool, []Finding{{Subject: "rolled-back", Kind: "usage_unavailable", Active: true}}, []nodeagent.Usage{sample}); err == nil {
		t.Fatal("invalid metric accepted")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM service_observations WHERE subject_key='rolled-back'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("usage scan failure not atomic")
	}
}

type usageObserver struct{ pressure, missing bool }

func (f usageObserver) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{MaximumSlots: 2, CPUs: 3, MemoryBytes: 4 << 30}, nil
}
func (f usageObserver) Status(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	return nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: true}, nil
}
func (f usageObserver) Usage(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Usage, error) {
	if f.missing {
		return nodeagent.Usage{}, errors.New("PRIVATE PATH OR KEY")
	}
	sample := nodeagent.Usage{InstanceID: id, DataBytes: 1000, BlobBytes: 68, DataFiles: 20, BlobFiles: 1, Accounts: 1, FilesystemBytes: 8 << 30, FilesystemAvailableBytes: 4 << 30, ObservedAtMillis: time.Now().UnixMilli()}
	if f.pressure {
		sample.Accounts = 2
		sample.FilesystemAvailableBytes = 128 << 20
	}
	return sample, nil
}
func TestUsageScanKeepsPressureFindingsDuringMeasurementOutage(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, node, id string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','usage-scan','Usage') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Usage',$1) RETURNING id::text`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id::text`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,'scan','local','scan','agent','ready','scan.localhost') RETURNING id::text`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		observer          usageObserver
		pressure, missing bool
	}{{usageObserver{pressure: true}, true, false}, {usageObserver{missing: true}, true, true}, {usageObserver{}, false, false}} {
		if _, err := Scan(ctx, pool, entry.observer); err != nil {
			t.Fatal(err)
		}
		for kind, expected := range map[string]bool{"disk_pressure": entry.pressure, "account_ceiling_exceeded": entry.pressure, "usage_unavailable": entry.missing} {
			var active bool
			if err := pool.QueryRow(ctx, `SELECT active FROM service_observations WHERE deployment_id=$1 AND kind=$2`, id, kind).Scan(&active); err != nil || active != expected {
				t.Fatalf("usage finding %s lost or cleared prematurely", kind)
			}
		}
		var bytes int64
		if err := pool.QueryRow(ctx, `SELECT data_bytes FROM instance_usage WHERE deployment_id=$1`, id).Scan(&bytes); err != nil || bytes != 1000 {
			t.Fatal("usage lost across outage")
		}
	}
}
