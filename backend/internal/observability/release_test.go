package observability

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestReleaseScanStoresOnlyMatchingPlacementAndClearsFailure(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node, other string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('fixture','release','Release') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES(gen_random_uuid(),$1,'Release') RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,'release','local','release','agent','ready','release.localhost') RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, target := range []*string{&node, &other} {
		if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local',$1,1,1,'active',$2,now()) RETURNING id`, []string{"https://localhost:9091", "https://localhost:9092"}[i], make([]byte, 32)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	runtime := nodeagent.ParseRuntimeRelease(nodeagent.PDSImage, "sha256:"+strings.Repeat("a", 64), "0.4.5036")
	observer := releaseObserver{runtime: runtime}
	if _, err := Scan(ctx, pool, observer); err != nil {
		t.Fatal(err)
	}
	value, err := DeploymentRelease(ctx, pool, id)
	if err != nil || value.State != "observed" || value.Runtime == nil || value.Runtime.Version != "0.4.5036" {
		t.Fatalf("scan: %+v %v", value, err)
	}
	// A response naming another instance must clear the observation.
	observer.foreign = true
	if _, err = Scan(ctx, pool, observer); err != nil {
		t.Fatal(err)
	}
	value, err = DeploymentRelease(ctx, pool, id)
	if err != nil || value.State != "unknown" || value.Runtime != nil || value.CheckedAt == nil {
		t.Fatal("foreign response accepted")
	}
	if err = recordScan(ctx, pool, nil, nil, []releaseSample{{id: id, node: other, runtime: runtime, checkedAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	value, err = DeploymentRelease(ctx, pool, id)
	if err != nil || value.Runtime != nil {
		t.Fatal("nonmatching placement wrote evidence")
	}
}

type releaseObserver struct {
	fixture
	runtime *nodeagent.RuntimeRelease
	foreign bool
}

func (f releaseObserver) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{MaximumSlots: 1, CPUs: 8, MemoryBytes: 8 << 30}, nil
}
func (f releaseObserver) Status(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	value := nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: true, Release: f.runtime}
	if f.foreign {
		value.ID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	}
	return value, nil
}
