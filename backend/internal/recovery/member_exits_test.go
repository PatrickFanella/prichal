package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

type memberExitObserver struct {
	observerFixture
	result  nodeagent.MemberExit
	failure bool
	calls   int
	node    string
}

func (o *memberExitObserver) MemberExitStatus(_ context.Context, peer nodeagent.Peer, id, ticket string) (nodeagent.MemberExit, error) {
	o.calls++
	if o.failure || peer.NodeID != o.node || id != o.result.InstanceID || ticket != o.result.Ticket {
		return nodeagent.MemberExit{}, errors.New("PRIVATE OBSERVATION FAILURE")
	}
	return o.result, nil
}

func TestRecoveryRequiresExactConfirmedMemberExitObservation(t *testing.T) {
	pool := testdb.NewRecovery(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var owner, org, id, node, ticket, removal string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','member-recovery','Recovery') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Recovery',$1) RETURNING id::text`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES($1,'members','local','members','agent','ready','members.localhost','https://members.localhost:44444') RETURNING id::text`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO hosting_nodes(region,agent_endpoint,slots,operator_slots,state,public_key_hash,enrolled_at) VALUES('local','https://127.0.0.1:9091',2,2,'active',$1,now()) RETURNING id::text`, make([]byte, 32)).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	did := "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"
	if err := pool.QueryRow(ctx, `INSERT INTO member_invitations(deployment_id,owner_id,idempotency_key,state,account_did,account_handle,message) VALUES($1,$2,gen_random_uuid(),'accepted',$3,'member.test','Accepted') RETURNING id::text`, id, owner, did).Scan(&ticket); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	deadline := start.Add(nodeagent.MemberMigrationWindow)
	if err := pool.QueryRow(ctx, `INSERT INTO member_removals(invitation_id,deployment_id,owner_id,requested_by,idempotency_key,account_did,state,message,host_requested_at,deactivate_after) VALUES($1,$2,$3,$3,gen_random_uuid(),$4,'migration_window','Retained',$5,$6) RETURNING id::text`, ticket, id, owner, did, start, deadline).Scan(&removal); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO member_removal_events(removal_id,state,message,actor_id) VALUES($1,'migration_window','Retained',$2)`, removal, owner); err != nil {
		t.Fatal(err)
	}
	if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	base := observerFixture{ids: []string{id}, healthy: true}
	observer := &memberExitObserver{observerFixture: base, node: node, result: nodeagent.MemberExit{InstanceID: id, Ticket: ticket, DID: did, State: "migration_window", RequestedAt: start.UnixMilli(), DeactivateAfter: deadline.UnixMilli()}}
	retained := func() string {
		var value string
		if err := pool.QueryRow(ctx, `SELECT row_to_json(r)::text || (SELECT json_agg(e ORDER BY e.id)::text FROM member_removal_events e WHERE e.removal_id=r.id) || (SELECT row_to_json(m)::text FROM member_invitations m WHERE m.id=r.invitation_id) FROM member_removals r WHERE r.id=$1`, removal).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	assertHeld := func(o Observer) {
		t.Helper()
		before := retained()
		if err := Reconcile(ctx, pool, o); err == nil {
			t.Fatal("ambiguous removal released hold")
		}
		if err := Check(ctx, pool); !errors.Is(err, ErrHeld) {
			t.Fatal("failed comparison changed hold", err)
		}
		if before != retained() {
			t.Fatal("recovery rewrote member intent, accepted identity or audit")
		}
	}
	assertHeld(base) // No inspection capability must not call a mutating fallback.
	observer.failure = true
	assertHeld(observer)
	observer.failure = false
	original := observer.result
	for _, mutate := range []func(*nodeagent.MemberExit){
		func(r *nodeagent.MemberExit) { r.DID = "did:plc:cccccccccccccccccccccccc" },
		func(r *nodeagent.MemberExit) { r.InstanceID = ticket },
		func(r *nodeagent.MemberExit) { r.Ticket = id },
		func(r *nodeagent.MemberExit) { r.State = "deactivated" },
		func(r *nodeagent.MemberExit) { r.RequestedAt++; r.DeactivateAfter++ },
		func(r *nodeagent.MemberExit) { r.DeactivateAfter-- },
	} {
		observer.result = original
		mutate(&observer.result)
		assertHeld(observer)
	}
	observer.result = original
	before := retained()
	if err := Reconcile(ctx, pool, observer); err != nil {
		t.Fatal(err)
	}
	if err := Check(ctx, pool); err != nil || before != retained() {
		t.Fatal("matching recovery changed removal metadata", err)
	}
	// Even a healthy matching host cannot release an uncertain request/review.
	for _, state := range []string{"requested", "review"} {
		if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE member_removals SET state=$2 WHERE id=$1`, removal, state); err != nil {
			t.Fatal(err)
		}
		calls := observer.calls
		assertHeld(observer)
		if observer.calls != calls {
			t.Fatal("uncertain removal dispatched inspection")
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE member_removals SET state='migration_window' WHERE id=$1`, removal); err != nil {
		t.Fatal(err)
	}
	// A missing placement cannot be exempted by an empty host inventory.
	if _, err := pool.Exec(ctx, `DELETE FROM instance_placements WHERE deployment_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	assertHeld(observerFixture{healthy: true})
	if _, err := pool.Exec(ctx, `INSERT INTO instance_placements(deployment_id,node_id) VALUES($1,$2)`, id, node); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE member_invitations SET account_did='did:plc:cccccccccccccccccccccccc' WHERE id=$1`, ticket); err != nil {
		t.Fatal(err)
	}
	assertHeld(observer)
	if _, err := pool.Exec(ctx, `UPDATE member_invitations SET account_did=$2 WHERE id=$1`, ticket, did); err != nil {
		t.Fatal(err)
	}
	var nextOwner string
	if err := pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local','changed-owner','Changed') RETURNING id::text`).Scan(&nextOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organizations SET owner_id=$2 WHERE id=$1`, org, nextOwner); err != nil {
		t.Fatal(err)
	}
	assertHeld(observer)
	if _, err := pool.Exec(ctx, `UPDATE organizations SET owner_id=$2 WHERE id=$1`, org, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE member_invitations SET state='expired',account_did=NULL,account_handle=NULL WHERE id=$1`, ticket); err != nil {
		t.Fatal(err)
	}
	assertHeld(observer)
	if _, err := pool.Exec(ctx, `UPDATE member_invitations SET state='accepted',account_did=$2,account_handle='member.test' WHERE id=$1`, ticket, did); err != nil {
		t.Fatal(err)
	}
	// Overdue windows are compared without completing or rescheduling anything.
	start = time.Now().Add(-nodeagent.MemberMigrationWindow - time.Hour).Truncate(time.Millisecond)
	deadline = start.Add(nodeagent.MemberMigrationWindow)
	if _, err := pool.Exec(ctx, `UPDATE member_removals SET host_requested_at=$2,deactivate_after=$3 WHERE id=$1`, removal, start, deadline); err != nil {
		t.Fatal(err)
	}
	observer.result.RequestedAt = start.UnixMilli()
	observer.result.DeactivateAfter = deadline.UnixMilli()
	before = retained()
	if err := Reconcile(ctx, pool, observer); err != nil || before != retained() {
		t.Fatal("overdue window advanced", err)
	}
	if err := Hold(ctx, pool, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE member_removals SET state='deactivated',completed_at=now() WHERE id=$1`, removal); err != nil {
		t.Fatal(err)
	}
	assertHeld(observer) // Host still reports the window rather than completion.
	observer.result.State = "deactivated"
	before = retained()
	if err := Reconcile(ctx, pool, observer); err != nil || before != retained() {
		t.Fatal("confirmed completion lost original metadata", err)
	}
}
