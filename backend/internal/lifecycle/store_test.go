package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/observability"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type fakeAgent struct {
	calls    int
	requests []nodeagent.ServiceRequest
	fail     bool
	status   nodeagent.Instance
}

func (a *fakeAgent) Service(_ context.Context, _ nodeagent.Peer, id string, r nodeagent.ServiceRequest) (nodeagent.ServiceResult, error) {
	a.calls++
	a.requests = append(a.requests, r)
	if a.fail {
		return nodeagent.ServiceResult{}, errors.New("PRIVATE HOST CREDENTIAL")
	}
	a.status = nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: r.State, Healthy: r.State == "running", ServiceGeneration: r.Generation, ServiceTicket: r.Ticket, ServicePhase: "applied"}
	return nodeagent.ServiceResult{InstanceID: id, ServiceRequest: r}, nil
}
func (a *fakeAgent) Status(context.Context, nodeagent.Peer, string) (nodeagent.Instance, error) {
	return a.status, nil
}
func (a *fakeAgent) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	return nodeagent.Capacity{MaximumSlots: 2, InstanceIDs: []string{a.status.ID}}, nil
}
func (a *fakeAgent) Usage(context.Context, nodeagent.Peer, string) (nodeagent.Usage, error) {
	return nodeagent.Usage{}, errors.New("usage unavailable")
}
func lifecycleJob(id string, attempt int) *river.Job[Args] {
	return &river.Job[Args]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 3}, Args: Args{id}}
}

func TestDurableOperatorServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node string
	for _, query := range []struct {
		sql  string
		args []any
		out  *string
	}{
		{`INSERT INTO owners(issuer,subject,display_name) VALUES('local','service','Service') RETURNING id::text`, nil, &owner},
	} {
		if err := pool.QueryRow(ctx, query.sql, query.args...).Scan(query.out); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Service',$1) RETURNING id::text`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname) VALUES($1,'service','local','service','agent','ready','service.localhost') RETURNING id::text`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id::text`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	r := Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", State: "suspended", Reason: "maintenance"}
	var wait sync.WaitGroup
	results := make(chan Operation, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			op, err := store.Submit(ctx, "local-operator", id, r)
			if err != nil {
				failures <- err
			} else {
				results <- op
			}
		}()
	}
	wait.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var op Operation
	for result := range results {
		if op.ID != "" && op.ID != result.ID {
			t.Fatal("duplicate intent")
		}
		op = result
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='service_lifecycle'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("intent and job not atomic")
	}
	if _, err = store.Submit(ctx, "different-operator", id, r); !errors.Is(err, ErrConflict) {
		t.Fatal("operator replay mismatch accepted")
	}
	conflict := r
	conflict.Key = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"
	conflict.State = "running"
	if _, err = store.Submit(ctx, "local-operator", id, conflict); !errors.Is(err, ErrConflict) {
		t.Fatal("pending transition allowed conflicting intent")
	}
	agent := &fakeAgent{}
	worker := &Worker{Pool: pool, Agent: agent}
	// Native success followed by a management checkpoint failure must replay the same generation.
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_service_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='applied' THEN RAISE EXCEPTION 'checkpoint failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER service_checkpoint BEFORE INSERT ON service_operation_events FOR EACH ROW EXECUTE FUNCTION reject_service_checkpoint()`); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, lifecycleJob(op.ID, 1)); err == nil {
		t.Fatal("checkpoint failure disappeared")
	}
	if _, err = pool.Exec(ctx, `DROP TRIGGER service_checkpoint ON service_operation_events`); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, lifecycleJob(op.ID, 2)); err != nil {
		t.Fatal(err)
	}
	if agent.calls != 2 || agent.requests[0] != agent.requests[1] {
		t.Fatal("uncertain checkpoint changed host intent")
	}
	if err = worker.Work(ctx, lifecycleJob(op.ID, 3)); err != nil || agent.calls != 2 {
		t.Fatal("completed replay executed host mutation")
	}
	findings, err := observability.Scan(ctx, pool, agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.DeploymentID == id && finding.Kind == "runtime_unhealthy" && finding.Active {
			t.Fatal("intentional suspension reported as outage")
		}
	}
	status, err := observability.DeploymentStatus(ctx, pool, id)
	if err != nil || status.State != "suspended" || status.Lifecycle != "suspended" {
		t.Fatal("customer suspension missing")
	}
	resume, err := store.Submit(ctx, "local-operator", id, conflict)
	if err != nil || resume.Generation != 2 || resume.PriorState != "suspended" {
		t.Fatal("resume generation incorrect")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, lifecycleJob(resume.ID, 1)); err == nil || agent.calls != 2 {
		t.Fatal("recovery hold executed host")
	}
	if _, err = pool.Exec(ctx, `UPDATE management_recovery SET state='reconciled',reconciled_at=now()`); err != nil {
		t.Fatal(err)
	}
	agent.fail = true
	if err = worker.Work(ctx, lifecycleJob(resume.ID, 3)); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("native error exposed or ignored")
	}
	failed, err := store.Get(ctx, resume.ID)
	if err != nil || failed.State != "failed" {
		t.Fatal("failure checkpoint missing")
	}
	conflict.Key = "cccccccc-cccc-4ccc-cccc-cccccccccccc"
	recover, err := store.Submit(ctx, "local-operator", id, conflict)
	if err != nil || recover.PriorState != "unknown" || recover.Generation != 3 {
		t.Fatal("uncertain state was treated as confirmed")
	}
	agent.fail = false
	if err = worker.Work(ctx, lifecycleJob(recover.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, lifecycleJob(resume.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if agent.status.ServiceGeneration != 3 || agent.status.State != "running" {
		t.Fatal("old work undid recovered state")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM service_operation_events WHERE operation_id=$1`, op.ID).Scan(&count); err != nil || count != 2 {
		t.Fatal("audit duplicated during checkpoint replay")
	}
}
