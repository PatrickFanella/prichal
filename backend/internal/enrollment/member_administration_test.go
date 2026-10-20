package enrollment

import (
	"context"
	"errors"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/orgaccess"
	"testing"
	"time"
)

func TestMemberAdministratorsRequireOptInAndCurrentRole(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	var org string
	if err := f.pool.QueryRow(ctx, `SELECT organization_id::text FROM deployments WHERE id=$1`, f.deployment).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) VALUES($1,$2,'administrator','active')`, org, f.bob); err != nil {
		t.Fatal(err)
	}
	invitation, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(800))
	if err != nil {
		t.Fatal(err)
	}
	denied := func() {
		t.Helper()
		if _, err := f.store.ListMembers(ctx, f.bob, f.deployment); !errors.Is(err, ErrNotFound) {
			t.Fatalf("list: %v", err)
		}
		if _, err := f.store.GetMember(ctx, f.bob, invitation.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("read: %v", err)
		}
		if _, err := f.store.SubmitMember(ctx, f.bob, f.deployment, memberKey(801)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("submit: %v", err)
		}
		if _, err := f.store.RefreshMember(ctx, f.bob, invitation.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("refresh: %v", err)
		}
		if _, err := f.store.WithdrawMember(ctx, f.bob, invitation.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("withdraw: %v", err)
		}
		if _, err := f.store.MemberAdmission(ctx, f.bob, f.deployment); !errors.Is(err, ErrNotFound) {
			t.Fatalf("policy: %v", err)
		}
	}
	denied() // Active administrator membership alone grants no native admission.
	f.store.MemberAdministrators = true
	created, err := f.store.SubmitMember(ctx, f.bob, f.deployment, memberKey(801))
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Events) != 1 || created.Events[0].ActorID == nil || *created.Events[0].ActorID != f.bob {
		t.Fatal("missing real actor")
	}
	var owner string
	if err = f.pool.QueryRow(ctx, `SELECT owner_id::text FROM member_invitations WHERE id=$1`, created.ID).Scan(&owner); err != nil || owner != f.owner {
		t.Fatal("administrator replaced canonical owner")
	}
	if _, err = f.store.GetMember(ctx, f.owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.MemberAdmission(ctx, f.bob, f.deployment); err != nil {
		t.Fatal(err)
	}
	if summary, err := f.store.ListMembers(ctx, f.bob, f.deployment); err != nil || len(summary.Invitations) != 2 {
		t.Fatalf("shared summary: %v", err)
	}
	for _, role := range []string{"viewer", "administrator"} {
		state := "active"
		if role == "administrator" {
			state = "revoked"
		}
		if _, err = f.pool.Exec(ctx, `UPDATE organization_members SET role=$1,state=$2 WHERE organization_id=$3 AND owner_id=$4`, role, state, org, f.bob); err != nil {
			t.Fatal(err)
		}
		denied()
	}

	if _, err = f.pool.Exec(ctx, `WITH other AS (INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Other organization',$1) RETURNING id) INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT id,$2,'administrator','active' FROM other`, f.owner, f.bob); err != nil {
		t.Fatal(err)
	}
	denied() // Administrator access to a different organization grants no scope here.

	if _, err = f.store.GetMember(ctx, f.owner, created.ID); err != nil {
		t.Fatal("owner lost access")
	}
}

func TestMemberAdministratorActionsPreserveWorkerOwnershipAndAudit(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	f.store.MemberAdministrators = true
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT organization_id,$1,'administrator','active' FROM deployments WHERE id=$2`, f.bob, f.deployment); err != nil {
		t.Fatal(err)
	}
	created, err := f.store.SubmitMember(ctx, f.bob, f.deployment, memberKey(810))
	if err != nil {
		t.Fatal(err)
	}
	agent := &memberAgentFixture{state: "invitation_ready"}
	worker := &MemberWorker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent, Queue: f.store.Queue}
	if err = worker.Work(ctx, memberJob(created.ID, 1)); err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.GetMember(ctx, f.bob, created.ID)
	if err != nil || ready.Code == "" {
		t.Fatalf("administrator code read: %v", err)
	}
	if _, err = f.store.RefreshMember(ctx, f.bob, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.WithdrawMember(ctx, f.bob, created.ID); err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, memberJob(created.ID, 1)); err != nil {
		t.Fatal(err)
	}
	result, err := f.store.GetMember(ctx, f.owner, created.ID)
	if err != nil || result.State != "withdrawn" || result.Code != "" {
		t.Fatalf("withdrawal: %v", err)
	}
	var actorEvents, workerEvents int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE actor_id=$2),count(*) FILTER(WHERE actor_id IS NULL) FROM member_invitation_events WHERE invitation_id=$1`, created.ID, f.bob).Scan(&actorEvents, &workerEvents); err != nil || actorEvents != 3 || workerEvents != 2 {
		t.Fatalf("audit actors %d worker %d: %v", actorEvents, workerEvents, err)
	}
}

func TestMemberPermissionSerializesWithRevocation(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	f.store.MemberAdministrators = true
	ctx := context.Background()
	var org string
	if err := f.pool.QueryRow(ctx, `SELECT organization_id::text FROM deployments WHERE id=$1`, f.deployment).Scan(&org); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) VALUES($1,$2,'administrator','active')`, org, f.bob); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = f.store.memberAccess(ctx, tx, f.bob, f.deployment, true); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		s := orgaccess.Store{Pool: f.pool, Enabled: true}
		done <- s.ChangeMember(ctx, f.owner, org, f.bob, "revoked")
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("revocation bypassed admitted action lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revocation stayed blocked")
	}
	if _, err = f.store.ListMembers(ctx, f.bob, f.deployment); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked actor retained access")
	}
}

func TestMemberActorMigrationDoesNotInventLegacyAttribution(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	invitation, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(820))
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct the exact pre-37 event shape inside this disposable database.
	if _, err = f.pool.Exec(ctx, `ALTER TABLE member_invitation_events DROP COLUMN actor_id; UPDATE schema_version SET version=36`); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Apply(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	result, err := f.store.GetMember(ctx, f.owner, invitation.ID)
	if err != nil || len(result.Events) != 1 || result.Events[0].ActorID != nil {
		t.Fatal("legacy actor inferred")
	}
	if _, err = f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(821)); err != nil {
		t.Fatal(err)
	}
}
