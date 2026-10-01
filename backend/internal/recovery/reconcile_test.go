package recovery

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type observerFixture struct {
	ids         []string
	healthy     bool
	unavailable bool
	service     *nodeagent.Instance
	account     *nodeagent.AccountStatus
}

func (o observerFixture) Capacity(context.Context, nodeagent.Peer) (nodeagent.Capacity, error) {
	if o.unavailable {
		return nodeagent.Capacity{}, errors.New("unavailable")
	}
	return nodeagent.Capacity{InstanceIDs: o.ids}, nil
}
func (o observerFixture) Status(_ context.Context, _ nodeagent.Peer, id string) (nodeagent.Instance, error) {
	if o.service != nil {
		return *o.service, nil
	}
	return nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "running", Healthy: o.healthy}, nil
}

func (o observerFixture) AccountStatus(context.Context, nodeagent.Peer, string) (nodeagent.AccountStatus, error) {
	if o.account == nil {
		return nodeagent.AccountStatus{}, errors.New("unavailable")
	}
	return *o.account, nil
}
func TestReconciliationPreservesHoldOnAmbiguity(t *testing.T) {
	pool := testdb.NewRecovery(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','recovery','Recovery') RETURNING id`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Recovery',$1) RETURNING id`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,'recovery','local','recovery','agent','checking','recovery.localhost','https://recovery.localhost:44444') RETURNING id`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES($1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []observerFixture{
		{ids: []string{id}, healthy: true, unavailable: true},
		{healthy: true},
		{ids: []string{id}, healthy: false},
		{ids: []string{id, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, healthy: true},
		{ids: []string{id, id}, healthy: true},
	} {
		if err := Reconcile(ctx, pool, fixture); err == nil {
			t.Fatal("ambiguous inventory released")
		}
		if err := Check(ctx, pool); !errors.Is(err, ErrHeld) {
			t.Fatalf("failed reconciliation changed hold: %v", err)
		}
	}
	extra := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	complete := observerFixture{ids: []string{id, extra}, healthy: true}
	if err := AcknowledgeRetained(ctx, pool, complete, node, id); err == nil {
		t.Fatal("managed instance exempted")
	}
	if err := AcknowledgeRetained(ctx, pool, observerFixture{}, node, extra); err == nil {
		t.Fatal("absent fixture acknowledged")
	}
	if err := AcknowledgeRetained(ctx, pool, complete, node, extra); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); !errors.Is(err, ErrHeld) {
		t.Fatal("acknowledgement released hold")
	}
	if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, pool, complete); err == nil {
		t.Fatal("new snapshot reused acknowledgement")
	}
	if err := AcknowledgeRetained(ctx, pool, complete, node, extra); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, pool, complete); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT state FROM deployments WHERE id=$1`, id).Scan(&state); err != nil || state != "checking" {
		t.Fatal("reconciliation advanced deployment")
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, healthy: true}); err == nil {
		t.Fatal("reconciliation ran without hold")
	}
	// A recovery copy must retain an intentional hold and match its exact host generation.
	if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	var operation string
	if err := pool.QueryRow(ctx, `INSERT INTO service_operations(deployment_id,idempotency_key,generation,desired_state,prior_state,reason,actor,state) VALUES($1,gen_random_uuid(),1,'suspended','running','maintenance','local-test','applied') RETURNING id::text`, id).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, healthy: true}); err == nil {
		t.Fatal("recovery ignored confirmed suspension")
	}
	held := nodeagent.Instance{ID: id, Endpoint: nodeagent.LocalEndpoint(id), State: "suspended", ServiceGeneration: 1, ServiceTicket: operation, ServicePhase: "applied"}
	wrong := held
	wrong.ServiceGeneration = 2
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, service: &wrong}); err == nil {
		t.Fatal("recovery ignored generation mismatch")
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, service: &held}); err != nil {
		t.Fatal(err)
	}
	if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE service_operations SET state='pending' WHERE id=$1`, operation); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, service: &held}); err == nil {
		t.Fatal("uncertain lifecycle released recovery hold")
	}
	if _, err := pool.Exec(ctx, `UPDATE service_operations SET state='applied',desired_state='running' WHERE id=$1`, operation); err != nil {
		t.Fatal(err)
	}
	running := held
	running.State = "running"
	running.Healthy = true
	var accountOp string
	if err := pool.QueryRow(ctx, `INSERT INTO service_operations(deployment_id,idempotency_key,generation,desired_state,prior_state,reason,actor,state,kind) VALUES($1,gen_random_uuid(),2,'suspended','running','safety_review','local-test','pending','account') RETURNING id::text`, id).Scan(&accountOp); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, service: &running}); err == nil {
		t.Fatal("pending moderation released hold")
	}
	if _, err := pool.Exec(ctx, `UPDATE service_operations SET state='applied' WHERE id=$1`, accountOp); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,state,account_did,account_handle,email_confirmed) VALUES($1,$2,gen_random_uuid(),'verified','did:plc:aaaaaaaaaaaaaaaaaaaaaaaa','first.test',true)`, id, owner); err != nil {
		t.Fatal(err)
	}
	account := nodeagent.AccountStatus{AccountResult: nodeagent.AccountResult{ServiceResult: nodeagent.ServiceResult{InstanceID: id, ServiceRequest: nodeagent.ServiceRequest{Ticket: accountOp, Generation: 2, State: "suspended"}}, DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"}, Phase: "applied"}
	wrongAccount := account
	wrongAccount.Generation = 3
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, service: &running, account: &wrongAccount}); err == nil {
		t.Fatal("moderation generation mismatch released hold")
	}
	if err := Reconcile(ctx, pool, observerFixture{ids: []string{id}, service: &running, account: &account}); err != nil {
		t.Fatal(err)
	}
}
