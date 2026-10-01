package orgaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

const alice = "11111111-1111-4111-8111-111111111111"
const bob = "22222222-2222-4222-8222-222222222222"
const stranger = "33333333-3333-4333-8333-333333333333"
const org = "44444444-4444-4444-8444-444444444444"
const key = "55555555-5555-4555-8555-555555555555"
const nextKey = "66666666-6666-4666-8666-666666666666"

func fixture(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{alice, bob, stranger} {
		if _, err := pool.Exec(ctx, `INSERT INTO owners(id,issuer,subject,display_name) VALUES($1::uuid,'http://localhost:5556',$1::text,'PRIVATE_EMAIL')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES($1,$2,'local collaboration')`, org, alice); err != nil {
		t.Fatal(err)
	}
	return &Store{pool, true, "http://localhost:5556"}
}
func accept(t *testing.T, s *Store, role string) Invitation {
	t.Helper()
	v, err := s.Invite(context.Background(), alice, org, bob, role, key)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Accept(context.Background(), bob, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestInvitationIdentitySingleUseAndImmediateRevocation(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	invitation, err := s.Invite(ctx, alice, org, bob, "viewer", key)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Invite(ctx, alice, org, bob, "viewer", key)
	if err != nil || retry.ID != invitation.ID || count(t, s, "organization_access_events") != 1 {
		t.Fatal("invite replay duplicated audit")
	}
	if _, err = s.Invite(ctx, alice, org, bob, "administrator", key); err != ErrConflict {
		t.Fatal("changed stable role accepted")
	}
	if _, err = s.Accept(ctx, stranger, invitation.ID); err != ErrNotFound {
		t.Fatal("foreign identity accepted invite")
	}
	if _, err = s.Shared(ctx, bob, org); err != ErrNotFound {
		t.Fatal("pending invitation granted access")
	}
	if _, err = s.Accept(ctx, bob, invitation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Accept(ctx, bob, invitation.ID); err != nil || count(t, s, "organization_access_events") != 2 {
		t.Fatal("accept replay duplicated audit")
	}
	shared, err := s.Shared(ctx, bob, org)
	if err != nil || strings.Contains(string(shared), "PRIVATE") || !strings.Contains(string(shared), "viewer") {
		t.Fatal("shared projection failed")
	}
	if err = s.Rename(ctx, bob, org, "unauthorized"); err != ErrNotFound {
		t.Fatal("viewer renamed organization")
	}
	if err = s.ChangeMember(ctx, bob, org, bob, "administrator"); err != ErrNotFound {
		t.Fatal("member escalated role")
	}
	if _, err = s.Manage(ctx, bob, org); err != ErrNotFound {
		t.Fatal("member inspected membership")
	}
	if err = s.ChangeMember(ctx, alice, org, bob, "revoked"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Shared(ctx, bob, org); err != ErrNotFound {
		t.Fatal("revocation delayed")
	}
	if _, err = s.Accept(ctx, bob, invitation.ID); err != nil {
		t.Fatal("receipt unavailable after revocation")
	}
	if _, err = s.Shared(ctx, bob, org); err != ErrNotFound {
		t.Fatal("accept replay regranted revoked membership")
	}
	if err = s.ChangeMember(ctx, alice, org, alice, "revoked"); err != ErrConflict {
		t.Fatal("last owner revoked")
	}
}
func TestExpiryIdentitySnapshotAndAuditRollback(t *testing.T) {
	for _, mode := range []string{"expired", "identity changed", "audit failure"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			ctx := context.Background()
			v, err := s.Invite(ctx, alice, org, bob, "viewer", key)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "expired":
				_, err = s.Pool.Exec(ctx, `UPDATE organization_invitations SET expires_at=now()-interval '1 second' WHERE id=$1`, v.ID)
			case "identity changed":
				_, err = s.Pool.Exec(ctx, `UPDATE owners SET subject='same display different identity' WHERE id=$1`, bob)
			case "audit failure":
				_, err = s.Pool.Exec(ctx, `ALTER TABLE organization_access_events ADD CONSTRAINT fixture_no_audit CHECK(false) NOT VALID`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Accept(ctx, bob, v.ID); err == nil {
				t.Fatal("invalid or unaudited acceptance escaped")
			}
			if count(t, s, "organization_members") != 0 {
				t.Fatal("failed accept committed membership")
			}
		})
	}
	s := fixture(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE owners SET issuer='http://localhost:5557' WHERE id=$1`, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Invite(ctx, alice, org, bob, "viewer", key); err != ErrConflict {
		t.Fatal("alternate issuer merged by display")
	}
}
func TestConcurrentInviteAndRoleMutation(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Invite(ctx, alice, org, bob, "viewer", key); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count(t, s, "organization_invitations") != 1 || count(t, s, "organization_access_events") != 1 {
		t.Fatal("concurrent invitation duplicated")
	}
	v, err := s.Invite(ctx, alice, org, bob, "viewer", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Accept(ctx, bob, v.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeMember(ctx, alice, org, bob, "administrator"); err != nil {
		t.Fatal(err)
	}
	if err = s.Rename(ctx, bob, org, "permitted"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Invite(ctx, bob, org, stranger, "viewer", nextKey); err != ErrNotFound {
		t.Fatal("administrator granted membership")
	}
	if err = s.ChangeMember(ctx, alice, org, bob, "viewer"); err != nil {
		t.Fatal(err)
	}
	if err = s.Rename(ctx, bob, org, "forbidden"); err != ErrNotFound {
		t.Fatal("demotion delayed")
	}
}
func TestSeparateHandoverConsentAndEmptyOrganizationFence(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	accept(t, s, "administrator")
	if _, err := s.Transfer(ctx, alice, org, bob, nextKey, false); err != ErrConflict {
		t.Fatal("unconfirmed handover prepared")
	}
	id, err := s.Transfer(ctx, alice, org, bob, nextKey, true)
	if err != nil {
		t.Fatal(err)
	}
	var owner string
	if err = s.Pool.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil || owner != alice {
		t.Fatal("offer changed ownership")
	}
	if _, err = s.AcceptTransfer(ctx, stranger, id, true); err != ErrNotFound {
		t.Fatal("foreign handover acceptance")
	}
	if _, err = s.AcceptTransfer(ctx, bob, id, false); err != ErrConflict {
		t.Fatal("recipient confirmation absent")
	}
	if _, err = s.AcceptTransfer(ctx, bob, id, true); err != nil {
		t.Fatal(err)
	}
	audits := count(t, s, "organization_access_events")
	if _, err = s.AcceptTransfer(ctx, bob, id, true); err != nil || count(t, s, "organization_access_events") != audits {
		t.Fatal("handover replay duplicated audit")
	}
	if _, err = s.Shared(ctx, alice, org); err != ErrNotFound {
		t.Fatal("former owner inherited membership")
	}
	v, err := s.Shared(ctx, bob, org)
	if err != nil || !strings.Contains(string(v), `"owner"`) {
		t.Fatal("new owner missing")
	}
	if err = s.ChangeMember(ctx, bob, org, bob, "viewer"); err != ErrConflict {
		t.Fatal("last owner demoted")
	}
}
func TestHandoverRechecksPopulationAndAudit(t *testing.T) {
	for _, mode := range []string{"populated", "revoked", "audit failure", "expired"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			ctx := context.Background()
			accept(t, s, "administrator")
			id, err := s.Transfer(ctx, alice, org, bob, nextKey, true)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "populated":
				_, err = s.Pool.Exec(ctx, `INSERT INTO deployments(organization_id,name,region,idempotency_key,managed_hostname) VALUES($1,'retained','local','retained','retained.localhost')`, org)
			case "revoked":
				err = s.ChangeMember(ctx, alice, org, bob, "revoked")
			case "audit failure":
				_, err = s.Pool.Exec(ctx, `ALTER TABLE organization_access_events ADD CONSTRAINT fixture_no_audit CHECK(false) NOT VALID`)
			case "expired":
				_, err = s.Pool.Exec(ctx, `UPDATE organization_transfers SET expires_at=now()-interval '1 second' WHERE id=$1`, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AcceptTransfer(ctx, bob, id, true); err == nil {
				t.Fatal("ineligible handover escaped")
			}
			var owner string
			if err = s.Pool.QueryRow(ctx, `SELECT owner_id FROM organizations WHERE id=$1`, org).Scan(&owner); err != nil || owner != alice {
				t.Fatal("failed handover lost owner")
			}
		})
	}
}

type sessions struct{ actor string }

func (s sessions) Authorize(w http.ResponseWriter, r *http.Request, mutation bool) (auth.Session, bool) {
	if mutation && r.Header.Get("X-CSRF-Token") != "fixture" {
		reply(w, 403, map[string]string{"error": "CSRF required"})
		return auth.Session{}, false
	}
	return auth.Session{OwnerID: s.actor}, true
}
func TestHTTPBoundariesAndDisabledDefault(t *testing.T) {
	s := fixture(t)
	mux := http.NewServeMux()
	s.Register(sessions{alice})(mux)
	body := `{"identityId":"` + bob + `","role":"viewer","idempotencyKey":"` + key + `"}`
	for _, tc := range []struct {
		method, path, body, csrf, auth string
		want                           int
	}{{"POST", "/api/team/organizations/" + org + "/invitations", body, "", "", 403}, {"POST", "/api/team/organizations/" + org + "/invitations", body, "fixture", "", 200}, {"GET", "/api/team/organizations", "", "", "Bearer private", 403}, {"POST", "/api/team/invitations/" + key + "/accept", `{"unknown":true}`, "fixture", "", 400}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.csrf != "" {
			r.Header.Set("X-CSRF-Token", tc.csrf)
		}
		if tc.auth != "" {
			r.Header.Set("Authorization", tc.auth)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("status %d want %d", w.Code, tc.want)
		}
		if strings.Contains(w.Body.String(), "PRIVATE") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response or cache")
		}
	}
	s.Enabled = false
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/team/organizations", nil))
	if w.Code != 404 {
		t.Fatal("default enabled")
	}
	t.Setenv("PDS_LOCAL_ORGANIZATION_ACCESS", "true")
	if enabled, err := LoadEnabled(false); err == nil || enabled {
		t.Fatal("production accepted rehearsal")
	}
	if enabled, err := LoadEnabled(true); err != nil || !enabled {
		t.Fatal("explicit local mode failed")
	}
	t.Setenv("PDS_LOCAL_ORGANIZATION_ACCESS", "")
	if enabled, err := LoadEnabled(true); err != nil || enabled {
		t.Fatal("default mode enabled")
	}
}
func TestSharedStatusDoesNotDelegatePrivateOwnerAPIs(t *testing.T) {
	s := fixture(t)
	accept(t, s, "viewer")
	if _, err := s.Pool.Exec(context.Background(), `INSERT INTO deployments(organization_id,name,region,idempotency_key,message,managed_hostname) VALUES($1,'status','local','status','PRIVATE_HOST_ERROR','status.localhost')`, org); err != nil {
		t.Fatal(err)
	}
	v, err := s.Shared(context.Background(), bob, org)
	if err != nil || strings.Contains(string(v), "PRIVATE") {
		t.Fatal("unsafe shared status")
	}
	var result struct {
		Instances []map[string]any `json:"instances"`
	}
	if json.Unmarshal(v, &result) != nil || len(result.Instances) != 1 || len(result.Instances[0]) != 4 {
		t.Fatal("projection widened")
	}
	if _, err = s.Transfer(context.Background(), alice, org, bob, nextKey, true); err != ErrConflict {
		t.Fatal("viewer or populated handover allowed")
	}
}
func TestExpiredInvitationReplacementAndRecoveryHold(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	prior, err := s.Invite(ctx, alice, org, bob, "viewer", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organization_invitations SET expires_at=now()-interval '1 second' WHERE id=$1`, prior.ID); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.Invite(ctx, alice, org, bob, "administrator", nextKey)
	if err != nil || fresh.ID == prior.ID {
		t.Fatal("expired invitation not replaced")
	}
	if _, err = s.Accept(ctx, bob, prior.ID); err != ErrConflict {
		t.Fatal("old invitation accepted after replacement")
	}
	if _, err = s.Accept(ctx, bob, fresh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO management_recovery(snapshot_sha256,state) VALUES(decode(repeat('00',32),'hex'),'held')`); err != nil {
		t.Fatal(err)
	}
	if err = s.Rename(ctx, bob, org, "held mutation"); err == nil {
		t.Fatal("recovery hold allowed rename")
	}
	if err = s.ChangeMember(ctx, alice, org, bob, "revoked"); err == nil {
		t.Fatal("recovery hold allowed revocation")
	}
	if _, err = s.Transfer(ctx, alice, org, bob, "77777777-7777-4777-8777-777777777777", true); err == nil {
		t.Fatal("recovery hold allowed handover offer")
	}
}
func TestOwnerMembershipProjectionAndRenameAuditRollback(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	accept(t, s, "administrator")
	view, err := s.Manage(ctx, alice, org)
	if err != nil || strings.Contains(string(view), "PRIVATE") || strings.Contains(string(view), "http://") {
		t.Fatal("owner projection unavailable or identity internals leaked")
	}
	var result struct {
		Members          []map[string]any `json:"members"`
		MembersTruncated bool             `json:"membersTruncated"`
	}
	if json.Unmarshal(view, &result) != nil || len(result.Members) != 1 || len(result.Members[0]) != 3 || result.MembersTruncated {
		t.Fatal("membership projection invalid")
	}
	if _, err = s.Pool.Exec(ctx, `ALTER TABLE organization_access_events ADD CONSTRAINT fixture_no_audit CHECK(false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if err = s.Rename(ctx, bob, org, "unaudited"); err == nil {
		t.Fatal("unaudited rename committed")
	}
	var name string
	if err = s.Pool.QueryRow(ctx, `SELECT name FROM organizations WHERE id=$1`, org).Scan(&name); err != nil || name != "local collaboration" {
		t.Fatal("failed rename changed organization")
	}
}
