package exports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/recovery"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const ownerID = "11111111-1111-4111-8111-111111111111"
const foreignID = "22222222-2222-4222-8222-222222222222"

func exportStoreFixture(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pool := testdb.NewRecovery(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{ownerID, foreignID} {
		if _, err := pool.Exec(ctx, `INSERT INTO owners(id,issuer,subject,display_name) VALUES($1,'local',$2,'Private owner')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	var org, node string
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Exports',$1) RETURNING id`, ownerID).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,$2,'exports','local','exports','agent','ready','exports.localhost')`, deploymentID, org); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,state,account_did,email_confirmed) VALUES($1,$2,gen_random_uuid(),'verified',$3,true)`, deploymentID, ownerID, fixtureDID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',3,3,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, deploymentID, node); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool, artifactFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type exportAgent struct {
	calls int
	fail  bool
	held  bool
	data  nodeagent.PortableExport
}

func (a *exportAgent) Status(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	state := "running"
	if a.held {
		state = "suspended"
	}
	return nodeagent.Instance{ID: id, State: state, Healthy: !a.held}, nil
}
func (a *exportAgent) Export(context.Context, nodeagent.Peer, string, string) (nodeagent.PortableExport, error) {
	a.calls++
	if a.fail {
		return nodeagent.PortableExport{}, errors.New("PRIVATE HOST CREDENTIAL")
	}
	return a.data, nil
}
func exportJob(id string, attempt int) *river.Job[Args] {
	return &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 3}, Args: Args{ExportID: id}}
}
func TestDurableOwnedExportAndLostCheckpoint(t *testing.T) {
	s := exportStoreFixture(t)
	ctx := context.Background()
	operation, err := s.Submit(ctx, ownerID, deploymentID, exportID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Submit(ctx, ownerID, deploymentID, exportID)
	if err != nil || replay.ID != operation.ID {
		t.Fatal("duplicate export intent")
	}
	if _, err = s.Get(ctx, foreignID, operation.ID); err != ErrNotFound {
		t.Fatal("foreign progress disclosed")
	}
	if _, err = s.Submit(ctx, foreignID, deploymentID, "cccccccc-cccc-4ccc-cccc-cccccccccccc"); err != ErrNotFound {
		t.Fatal("foreign capture accepted")
	}
	if _, err = s.List(ctx, foreignID, deploymentID); err != ErrNotFound {
		t.Fatal("foreign list disclosed")
	}
	listed, err := s.List(ctx, ownerID, deploymentID)
	if err != nil || len(listed) != 1 || listed[0].ID != operation.ID {
		t.Fatal("owned list lost durable request")
	}
	agent := &exportAgent{data: portableFixture()}
	worker := &Worker{Store: s, Agent: agent}
	if err = worker.Work(ctx, exportJob(operation.ID, 1)); err != nil {
		t.Fatal(err)
	}
	ready, err := s.Get(ctx, ownerID, operation.ID)
	if err != nil || ready.State != "ready" || !ready.DownloadAvailable {
		t.Fatal("export did not become available")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE export_operations SET expires_at=NULL WHERE id=$1`, operation.ID); err == nil {
		t.Fatal("ready export without expiry accepted")
	}
	download, err := s.Download(ctx, ownerID, operation.ID)
	if err != nil || digest(download) != *ready.SHA256 {
		t.Fatal("owned download failed")
	}
	if plain, err := s.Download(ctx, foreignID, operation.ID); err != ErrNotFound || plain != nil {
		t.Fatal("foreign download disclosed")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE export_operations SET state='queued',archive_bytes=NULL,archive_sha256=NULL,captured_at=NULL,expires_at=NULL WHERE id=$1`, operation.ID); err != nil {
		t.Fatal(err)
	}
	agent.data.CAR = []byte("changed source")
	if err = worker.Work(ctx, exportJob(operation.ID, 2)); err != nil {
		t.Fatal(err)
	}
	same, err := s.Download(ctx, ownerID, operation.ID)
	if err != nil || digest(same) != digest(download) || agent.calls != 1 {
		t.Fatal("lost checkpoint recaptured or changed data")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE export_operations SET captured_at=now()-interval '25 hours',expires_at=now()-interval '1 hour' WHERE id=$1`, operation.ID); err != nil {
		t.Fatal(err)
	}
	if plain, err := s.Download(ctx, ownerID, operation.ID); err == nil || plain != nil {
		t.Fatal("expired download disclosed")
	}
	if _, _, err = s.Artifacts.Read(operation.ID, deploymentID, fixtureDID); err != nil {
		t.Fatal("expiry removed retained data")
	}
}
func TestRetainedExportAllowanceSerializesConcurrentRequests(t *testing.T) {
	s := exportStoreFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Submit(ctx, ownerID, deploymentID, fmt.Sprintf("%08x-cccc-4ccc-cccc-cccccccccccc", i+1))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	accepted, limited := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrLimit) {
			limited++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 3 || limited != 5 {
		t.Fatal("retained request limit failed")
	}
	var jobs int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='portable_export'`).Scan(&jobs); err != nil || jobs != 3 {
		t.Fatal("intent/job transaction mismatch")
	}
}
func TestExportQueueFailureAndPrivateCaptureFailure(t *testing.T) {
	s := exportStoreFixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `ALTER TABLE river_job ADD CONSTRAINT fixture_export_queue_failure CHECK(false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, ownerID, deploymentID, exportID); err == nil {
		t.Fatal("queue failure accepted")
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM export_operations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("uncommitted intent remained")
	}
	if _, err := s.Pool.Exec(ctx, `ALTER TABLE river_job DROP CONSTRAINT fixture_export_queue_failure`); err != nil {
		t.Fatal(err)
	}
	operation, err := s.Submit(ctx, ownerID, deploymentID, exportID)
	if err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Store: s, Agent: &exportAgent{fail: true, data: portableFixture()}}
	err = worker.Work(ctx, exportJob(operation.ID, 3))
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("private failure leaked")
	}
	failed, err := s.Get(ctx, ownerID, operation.ID)
	if err != nil || failed.State != "failed" {
		t.Fatal("terminal outcome absent")
	}
}
func TestSuspendedAndRecoveryHeldExportsWaitWithoutDeletingData(t *testing.T) {
	s := exportStoreFixture(t)
	ctx := context.Background()
	operation, err := s.Submit(ctx, ownerID, deploymentID, exportID)
	if err != nil {
		t.Fatal(err)
	}
	agent := &exportAgent{held: true, data: portableFixture()}
	worker := &Worker{Store: s, Agent: agent}
	if err = worker.Work(ctx, exportJob(operation.ID, 1)); err == nil || agent.calls != 0 {
		t.Fatal("suspended capture ran")
	}
	if err = recovery.Hold(ctx, s.Pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, exportJob(operation.ID, 1)); err == nil || agent.calls != 0 {
		t.Fatal("held management capture ran")
	}
}
