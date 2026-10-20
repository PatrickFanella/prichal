package enrollment

import (
	"context"
	"errors"
	"testing"
)

func TestAcceptedMemberRosterSurvivesInvitationHistoryWindow(t *testing.T) {
	f := newStoreFixture(t)
	f.ownerAccount(t)
	ctx := context.Background()
	var accepted string
	if err := f.pool.QueryRow(ctx, `INSERT INTO member_invitations(deployment_id,owner_id,idempotency_key,state,account_did,account_handle,message,created_at) VALUES($1,$2,gen_random_uuid(),'accepted','did:plc:bbbbbbbbbbbbbbbbbbbbbbbb','older-member.test','Accepted member',now()-interval '10 days') RETURNING id::text`, f.deployment, f.owner).Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO member_invitations(deployment_id,owner_id,idempotency_key,state,message,created_at) SELECT $1,$2,gen_random_uuid(),'expired','Expired fixture',now()-n*interval '1 minute' FROM generate_series(1,105) n`, f.deployment, f.owner); err != nil {
		t.Fatal(err)
	}
	summary, err := f.store.ListMembers(ctx, f.owner, f.deployment)
	if err != nil || summary.Members != 1 || len(summary.Accounts) != 1 || len(summary.Invitations) != 100 {
		t.Fatalf("roster: %+v %v", summary, err)
	}
	if summary.Accounts[0].InvitationID != accepted || summary.Accounts[0].Handle != "older-member.test" || summary.Accounts[0].DID != "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatal("older accepted member missing")
	}
	for _, invitation := range summary.Invitations {
		if invitation.ID == accepted {
			t.Fatal("fixture did not exceed history window")
		}
	}
	if _, err = f.store.ListMembers(ctx, f.bob, f.deployment); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner read roster")
	}
	f.store.MemberAdministrators = true
	if _, err = f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT organization_id,$1,'administrator','active' FROM deployments WHERE id=$2`, f.bob, f.deployment); err != nil {
		t.Fatal(err)
	}
	shared, err := f.store.ListMembers(ctx, f.bob, f.deployment)
	if err != nil || len(shared.Accounts) != 1 || shared.Accounts[0] != summary.Accounts[0] {
		t.Fatal("administrator roster differs")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE organization_members SET state='revoked' WHERE owner_id=$1`, f.bob); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ListMembers(ctx, f.bob, f.deployment); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked member read roster")
	}
}
