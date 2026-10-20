package enrollment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type memberAgentFixture struct {
	calls, withdrawals int
	state              string
	failure            bool
	otherTicket        bool
	did, handle        string
}

func (a *memberAgentFixture) reply(id, ticket string, withdraw bool) (nodeagent.MemberInvitation, error) {
	if a.failure {
		return nodeagent.MemberInvitation{}, errors.New("PRIVATE ADMINISTRATOR KEY")
	}
	if a.otherTicket {
		ticket = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	}
	result := nodeagent.MemberInvitation{InstanceID: id, Ticket: ticket, State: a.state}
	switch a.state {
	case "invitation_ready":
		result.Code, result.ExpiresAt = "member-"+ticket, time.Now().Add(nodeagent.MemberInvitationLifetime).UnixMilli()
	case "accepted":
		result.DID, result.Handle = a.did, a.handle
	case "expired":
		result.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	}
	if withdraw && a.state == "invitation_ready" {
		result = nodeagent.MemberInvitation{InstanceID: id, Ticket: ticket, State: "withdrawn"}
	}
	return result, nil
}
func (a *memberAgentFixture) MemberInvitation(_ context.Context, _ nodeagent.Peer, id, ticket string) (nodeagent.MemberInvitation, error) {
	a.calls++
	return a.reply(id, ticket, false)
}
func (a *memberAgentFixture) WithdrawMemberInvitation(_ context.Context, _ nodeagent.Peer, id, ticket string) (nodeagent.MemberInvitation, error) {
	a.withdrawals++
	return a.reply(id, ticket, true)
}
func memberJob(id string, attempt int) *river.Job[MemberArgs] {
	return &river.Job[MemberArgs]{JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 3}, Args: MemberArgs{id}}
}
func memberKey(n int) MemberRequest {
	return MemberRequest{Key: fmt.Sprintf("%08d-aaaa-4aaa-8aaa-aaaaaaaaaaaa", n)}
}
func (f storeFixture) ownerAccount(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO enrollment_operations(deployment_id,owner_id,idempotency_key,state,account_did,account_handle,email_confirmed,message) VALUES($1,$2,gen_random_uuid(),'verified','did:plc:aaaaaaaaaaaaaaaaaaaaaaaa','owner.test',true,'verified')`, f.deployment, f.owner); err != nil {
		t.Fatal(err)
	}
}

func TestMemberInvitationIntentIsGatedOwnerScopedAndIdempotent(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(1)); !errors.Is(err, ErrOwnerAccountRequired) {
		t.Fatalf("member invited before owner account: %v", err)
	}
	f.ownerAccount(t)
	var wait sync.WaitGroup
	results := make(chan MemberInvitation, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			invitation, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(1))
			if err != nil {
				failures <- err
			} else {
				results <- invitation
			}
		}()
	}
	wait.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var id string
	for result := range results {
		if id == "" {
			id = result.ID
		}
		if result.ID != id || result.State != "pending" || result.Code != "" {
			t.Fatalf("duplicate or unexpected intent: %+v", result)
		}
	}
	var jobs, events int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM river_job WHERE kind='member_invitation'),(SELECT count(*) FROM member_invitation_events)`).Scan(&jobs, &events); err != nil || jobs != 1 || events != 1 {
		t.Fatalf("intent, event and job were not atomic: %d %d %v", jobs, events, err)
	}
	if _, err := f.store.GetMember(ctx, f.bob, id); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner read member invitation")
	}
	if _, err := f.store.SubmitMember(ctx, f.bob, f.deployment, memberKey(2)); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner created member invitation")
	}
	if _, err := f.store.ListMembers(ctx, f.bob, f.deployment); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner listed members")
	}

	// Paused admission refuses new intent; an existing request still replays.
	if _, err := f.store.SetAdmission(ctx, "operator", "local_qualification", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(2)); !errors.Is(err, ErrAdmissionClosed) {
		t.Fatalf("closed admission accepted member intent: %v", err)
	}
	if replay, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(1)); err != nil || replay.ID != id {
		t.Fatal("closed admission blocked replay")
	}
	if _, err := f.store.SetAdmission(ctx, "operator", "local_qualification", true); err != nil {
		t.Fatal(err)
	}

	// Ten new requests per hour per deployment.
	for n := 2; n <= 10; n++ {
		if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(n)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(11)); !errors.Is(err, ErrConflict) {
		t.Fatal("hourly member limit not enforced")
	}

	// Accepted members and open invitations count toward the ceiling.
	if _, err := f.pool.Exec(ctx, `UPDATE member_invitations SET created_at=now()-interval '2 hours'`); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < nodeagent.MemberCeiling-10; n++ {
		did := "did:plc:" + strings.Repeat("b", 22) + string(rune('a'+n/26)) + string(rune('a'+n%26))
		if _, err := f.pool.Exec(ctx, `INSERT INTO member_invitations(deployment_id,owner_id,idempotency_key,state,account_did,account_handle,message,created_at) VALUES($1,$2,gen_random_uuid(),'accepted',$3,'member.test','accepted',now()-interval '2 hours')`, f.deployment, f.owner, did); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(12)); !errors.Is(err, ErrMemberCeiling) {
		t.Fatalf("member ceiling not enforced: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE member_invitations SET state='withdrawn',withdrawal_requested_at=now(),account_did=NULL,account_handle=NULL WHERE state='accepted'`); err != nil {
		t.Fatal(err)
	}

	// An active hosting exit refuses new members.
	var export string
	if err := f.pool.QueryRow(ctx, `INSERT INTO export_operations(deployment_id,owner_id,idempotency_key,account_did) VALUES($1,$2,gen_random_uuid(),'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa') RETURNING id::text`, f.deployment, f.owner).Scan(&export); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO hosting_exits(deployment_id,owner_id,idempotency_key,export_id,service_end_at,retention_review_at,delay_seconds,retention_days) VALUES($1,$2,gen_random_uuid(),$3,date_trunc('second',now())+interval '1 hour',date_trunc('second',now())+interval '25 hours',3600,1)`, f.deployment, f.owner, export); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(13)); !errors.Is(err, ErrHostingExit) {
		t.Fatalf("hosting exit did not refuse members: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE hosting_exits SET state='reversed'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(13)); err != nil {
		t.Fatalf("reversed exit still refused members: %v", err)
	}
}

func TestMemberWorkerIssuesSchedulesExpiryAndRecordsAcceptance(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	f.ownerAccount(t)
	agent := &memberAgentFixture{state: "invitation_ready"}
	worker := &MemberWorker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent, Queue: f.store.Queue}
	invitation, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(1))
	if err != nil {
		t.Fatal(err)
	}
	if err = (&MemberWorker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent}).Work(ctx, memberJob(invitation.ID, 1)); err == nil {
		t.Fatal("worker without a queue client reported success")
	}
	if err = worker.Work(ctx, memberJob(invitation.ID, 1)); err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.GetMember(ctx, f.owner, invitation.ID)
	if err != nil || ready.State != "invitation_ready" || ready.Code != "member-"+invitation.ID || ready.ExpiresAt == nil || ready.AccountURL != nodeagent.LocalEndpoint(f.deployment)+"/account" {
		t.Fatalf("ready invitation: %+v %v", ready, err)
	}
	var scheduled time.Time
	if err = f.pool.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind='member_invitation' ORDER BY id DESC LIMIT 1`).Scan(&scheduled); err != nil || scheduled.Before(ready.ExpiresAt.Add(time.Second)) {
		t.Fatalf("expiry job not scheduled after the native window: %v %v", scheduled, err)
	}
	var stored []byte
	if err = f.pool.QueryRow(ctx, `SELECT code_ciphertext FROM member_invitations WHERE id=$1`, invitation.ID).Scan(&stored); err != nil || strings.Contains(string(stored), "member-") {
		t.Fatal("member code stored without encryption")
	}
	if _, err = f.store.GetMember(ctx, f.bob, invitation.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner read a member code")
	}
	if _, err = f.store.RefreshMember(ctx, f.owner, invitation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.RefreshMember(ctx, f.owner, invitation.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("refresh was not rate limited")
	}

	// The member signs up natively; the next check records public identity only.
	agent.state, agent.did, agent.handle = "accepted", "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb", "member.test"
	if err = worker.Work(ctx, memberJob(invitation.ID, 1)); err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.GetMember(ctx, f.owner, invitation.ID)
	if err != nil || accepted.State != "accepted" || accepted.Code != "" || accepted.DID != agent.did || accepted.Handle != "member.test" || len(accepted.Events) != 4 {
		t.Fatalf("accepted member: %+v %v", accepted, err)
	}
	if accepted.Events[2].ActorID == nil || *accepted.Events[2].ActorID != f.owner || accepted.Events[2].Message != "Native member invitation check requested." || accepted.Events[3].ActorID != nil {
		t.Fatal("refresh and worker attribution differ")
	}

	summary, err := f.store.ListMembers(ctx, f.owner, f.deployment)
	if err != nil || summary.Members != 1 || summary.Open != 0 || summary.Ceiling != nodeagent.MemberCeiling || len(summary.Invitations) != 1 {
		t.Fatalf("member summary: %+v %v", summary, err)
	}
	calls := agent.calls
	if err = worker.Work(ctx, memberJob(invitation.ID, 1)); err != nil || agent.calls != calls {
		t.Fatal("terminal invitation contacted the node again")
	}
	if _, err = f.store.WithdrawMember(ctx, f.owner, invitation.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted invitation was withdrawn")
	}

	// The same native account cannot be accepted twice.
	other, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(2))
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(ctx, memberJob(other.ID, 1)); err == nil {
		t.Fatal("duplicate member identity recorded")
	}

	// A node answer for another ticket is rejected.
	agent.state, agent.otherTicket = "invitation_ready", true
	if err = worker.Work(ctx, memberJob(other.ID, 1)); err == nil {
		t.Fatal("foreign ticket accepted")
	}
}

func TestMemberWithdrawalExpiryAndFailureKeepNativeTruth(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	f.ownerAccount(t)
	agent := &memberAgentFixture{state: "invitation_ready"}
	worker := &MemberWorker{Pool: f.pool, Cipher: f.store.Cipher, Agent: agent, Queue: f.store.Queue}
	invitation, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.WithdrawMember(ctx, f.owner, invitation.ID); err != nil {
		t.Fatal("pending withdrawal should replay current status")
	}
	if err = worker.Work(ctx, memberJob(invitation.ID, 1)); err != nil {
		t.Fatal(err)
	}
	withdrawing, err := f.store.WithdrawMember(ctx, f.owner, invitation.ID)
	if err != nil || withdrawing.State != "withdrawing" || withdrawing.Code != "" || !withdrawing.WithdrawalRequested {
		t.Fatalf("withdrawal intent: %+v %v", withdrawing, err)
	}
	if again, err := f.store.WithdrawMember(ctx, f.owner, invitation.ID); err != nil || again.State != "withdrawing" {
		t.Fatal("withdrawal replay")
	}
	if _, err = f.store.WithdrawMember(ctx, f.bob, invitation.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign owner withdrew an invitation")
	}
	if err = worker.Work(ctx, memberJob(invitation.ID, 1)); err != nil {
		t.Fatal(err)
	}
	withdrawn, err := f.store.GetMember(ctx, f.owner, invitation.ID)
	if err != nil || withdrawn.State != "withdrawn" || agent.withdrawals != 1 {
		t.Fatalf("withdrawn: %+v %v", withdrawn, err)
	}

	// A code used just before withdrawal stays an accepted member.
	raced, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(2))
	if err != nil || worker.Work(ctx, memberJob(raced.ID, 1)) != nil {
		t.Fatal(err)
	}
	if _, err = f.store.WithdrawMember(ctx, f.owner, raced.ID); err != nil {
		t.Fatal(err)
	}
	agent.state, agent.did, agent.handle = "accepted", "did:plc:cccccccccccccccccccccccc", "raced.test"
	if err = worker.Work(ctx, memberJob(raced.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if member, err := f.store.GetMember(ctx, f.owner, raced.ID); err != nil || member.State != "accepted" || !member.WithdrawalRequested {
		t.Fatalf("raced member: %+v %v", member, err)
	}

	// The scheduled expiry job records the node's expiry.
	expiring, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(3))
	agent.state = "invitation_ready"
	if err != nil || worker.Work(ctx, memberJob(expiring.ID, 1)) != nil {
		t.Fatal(err)
	}
	agent.state = "expired"
	if err = worker.Work(ctx, memberJob(expiring.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if expired, err := f.store.GetMember(ctx, f.owner, expiring.ID); err != nil || expired.State != "expired" || expired.Code != "" {
		t.Fatalf("expired: %+v %v", expired, err)
	}

	// An issued invitation cannot regress to a refusal.
	issued, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(4))
	agent.state = "invitation_ready"
	if err != nil || worker.Work(ctx, memberJob(issued.ID, 1)) != nil {
		t.Fatal(err)
	}
	agent.state = "ceiling_reached"
	if err = worker.Work(ctx, memberJob(issued.ID, 1)); err == nil {
		t.Fatal("issued invitation regressed")
	}

	// Final failure records a bounded message, never the node error.
	failing, err := f.store.SubmitMember(ctx, f.owner, f.deployment, memberKey(5))
	if err != nil {
		t.Fatal(err)
	}
	agent.failure = true
	for attempt := 1; attempt <= 3; attempt++ {
		if err = worker.Work(ctx, memberJob(failing.ID, attempt)); err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("native failure leaked or succeeded")
		}
	}
	failed, err := f.store.GetMember(ctx, f.owner, failing.ID)
	if err != nil || failed.State != "failed" || strings.Contains(failed.Message, "PRIVATE") {
		t.Fatalf("failure: %+v %v", failed, err)
	}
	// Failure does not close an issued invitation; native expiry stays authoritative.
	if err = worker.Work(ctx, memberJob(issued.ID, 3)); err == nil {
		t.Fatal("failed refresh reported success")
	}
	if open, err := f.store.GetMember(ctx, f.owner, issued.ID); err != nil || open.State != "invitation_ready" {
		t.Fatalf("failed refresh closed an issued invitation: %+v %v", open, err)
	}
}

func TestMemberHTTPEnforcesOwnerCSRFAndCodeBoundary(t *testing.T) {
	f := newStoreFixture(t)
	ctx := context.Background()
	f.ownerAccount(t)
	issuerServer := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + issuerServer.Listener.Addr().String()
	issuer, err := testissuer.New(issuerURL, "test", "http://localhost/api/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	issuerServer.Config.Handler = issuer.Handler()
	issuerServer.Start()
	defer issuerServer.Close()
	owners, err := auth.New(ctx, f.pool, auth.Settings{Issuer: issuerURL, ClientID: "test", Origin: "http://localhost", Mode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	aliceToken, bobToken := strings.Repeat("a", 43), strings.Repeat("b", 43)
	for _, entry := range []struct{ token, owner string }{{aliceToken, f.owner}, {bobToken, f.bob}} {
		hash := sha256.Sum256([]byte(entry.token))
		if _, err = f.pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'test-csrf',now()+interval '1 hour')`, hash[:], entry.owner); err != nil {
			t.Fatal(err)
		}
	}
	request := func(local bool, method, path, token, origin, csrf, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		f.store.RegisterMembers(owners, local)(mux)
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", csrf)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "pds-local-session", Value: token})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	path := "/api/deployments/" + f.deployment + "/members/invitations"
	body := `{"idempotencyKey":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`
	for _, entry := range []struct {
		token, origin, csrf, body string
		status                    int
	}{
		{"", "http://localhost", "", body, 401},
		{aliceToken, "http://localhost", "wrong", body, 403},
		{aliceToken, "http://foreign.invalid", "test-csrf", body, 403},
		{bobToken, "http://localhost", "test-csrf", body, 404},
		{aliceToken, "http://localhost", "test-csrf", `{"idempotencyKey":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","password":"PRIVATE PDS PASSWORD"}`, 400},
		{aliceToken, "http://localhost", "test-csrf", body, 200},
	} {
		w := request(true, "POST", path, entry.token, entry.origin, entry.csrf, entry.body)
		if w.Code != entry.status || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("owner boundary: %d %s", w.Code, w.Body.String())
		}
	}
	if w := request(false, "GET", path, aliceToken, "", "", ""); w.Code != 503 {
		t.Fatal("member routes served outside local mode")
	}
	if w := request(true, "GET", path, aliceToken, "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"ceiling":25`) {
		t.Fatalf("member list: %d %s", w.Code, w.Body.String())
	}
	if w := request(true, "GET", path, bobToken, "", "", ""); w.Code != 404 {
		t.Fatal("foreign owner listed members")
	}

	if _, err = f.pool.Exec(ctx, `INSERT INTO organization_members(organization_id,owner_id,role,state) SELECT organization_id,$1,'administrator','active' FROM deployments WHERE id=$2`, f.bob, f.deployment); err != nil {
		t.Fatal(err)
	}
	f.store.MemberAdministrators = true
	if w := request(true, "GET", path, bobToken, "", "", ""); w.Code != 200 {
		t.Fatalf("administrator list: %d", w.Code)
	}
	if w := request(true, "GET", "/api/deployments/"+f.deployment+"/members/admission", bobToken, "", "", ""); w.Code != 200 {
		t.Fatalf("administrator policy: %d", w.Code)
	}
	if w := request(true, "POST", path, bobToken, "http://localhost", "wrong", body); w.Code != 403 {
		t.Fatal("administrator bypassed CSRF")
	}
	if w := request(true, "POST", path, bobToken, "http://localhost", "test-csrf", `{"idempotencyKey":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}`); w.Code != 200 {
		t.Fatalf("administrator submit: %d", w.Code)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE organization_members SET state='revoked' WHERE owner_id=$1`, f.bob); err != nil {
		t.Fatal(err)
	}
	if w := request(true, "GET", path, bobToken, "", "", ""); w.Code != 404 {
		t.Fatal("revoked administrator retained HTTP access")
	}

	accepted := acceptedRemovalMember(t, f, 950, "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	removalPath := "/api/member-invitations/" + accepted.ID + "/removal"
	for _, entry := range []struct {
		token, origin, csrf, body string
		status                    int
	}{
		{"", "http://localhost", "", body, 401},
		{aliceToken, "http://localhost", "wrong", body, 403},
		{aliceToken, "http://foreign.invalid", "test-csrf", body, 403},
		{bobToken, "http://localhost", "test-csrf", body, 404},
		{aliceToken, "http://localhost", "test-csrf", `{"idempotencyKey":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","did":"did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"}`, 400},
		{aliceToken, "http://localhost", "test-csrf", `{"idempotencyKey":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","deactivateAfter":1}`, 400},
		{aliceToken, "http://localhost", "test-csrf", body + "{}", 400},
		{aliceToken, "http://localhost", "test-csrf", body, 200},
	} {
		w := request(true, "POST", removalPath, entry.token, entry.origin, entry.csrf, entry.body)
		if w.Code != entry.status {
			t.Fatalf("removal HTTP boundary: %d %s", w.Code, w.Body.String())
		}
	}
	if w := request(true, "GET", removalPath, aliceToken, "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"requested"`) {
		t.Fatalf("removal read: %d %s", w.Code, w.Body.String())
	}
	if w := request(true, "GET", removalPath, bobToken, "", "", ""); w.Code != 404 {
		t.Fatal("revoked actor read removal")
	}
	if w := request(false, "POST", removalPath, aliceToken, "http://localhost", "test-csrf", body); w.Code != 503 {
		t.Fatal("removal admitted outside local mode")
	}

	if w := request(true, "POST", "/api/member-invitations/not-a-uuid/withdraw", aliceToken, "http://localhost", "test-csrf", `{}`); w.Code != 404 {
		t.Fatal("invalid invitation identity accepted")
	}
}
