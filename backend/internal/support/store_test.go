package support

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/lifecycle"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

const alice = "11111111-1111-4111-8111-111111111111"
const bob = "22222222-2222-4222-8222-222222222222"
const otherIssuer = "33333333-3333-4333-8333-333333333333"
const organization = "44444444-4444-4444-8444-444444444444"
const instance = "55555555-5555-4555-8555-555555555555"

func fixture(t *testing.T) (*Store, *lifecycle.Store) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ id, issuer, subject string }{{alice, "http://localhost:5556", "alice"}, {bob, "http://localhost:5556", "bob"}, {otherIssuer, "http://localhost:5557", "alice"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO owners(id,issuer,subject,display_name) VALUES($1,$2,$3,'PRIVATE_EMAIL_AND_DISPLAY')`, entry.id, entry.issuer, entry.subject); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES($1,$2,'local org')`, organization, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,state,kind,managed_hostname,message) VALUES($1,$2,'fixture','local','fixture','ready','agent','fixture.localhost','PRIVATE_HOST_ERROR')`, instance, organization); err != nil {
		t.Fatal(err)
	}
	actions, err := lifecycle.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	return &Store{Pool: pool, Settings: Settings{Identities: []Identity{{"http://localhost:5556", "alice", "operate"}}}}, actions
}
func TestExactOperatorIdentityAndSafeAuditedView(t *testing.T) {
	s, actions := fixture(t)
	ctx := context.Background()
	for _, tc := range []struct{ owner, role string }{{alice, "operate"}, {bob, ""}, {otherIssuer, ""}} {
		role, err := s.role(ctx, tc.owner)
		if err != nil || role != tc.role {
			t.Fatal("issuer/subject role mismatch")
		}
	}
	op, err := actions.Submit(ctx, "oidc-owner:"+alice, instance, lifecycle.Request{Key: "66666666-6666-4666-8666-666666666666", Kind: "account", State: "suspended", Reason: "safety_review"})
	if err != nil {
		t.Fatal(err)
	}
	organizationView, err := s.Organization(ctx, alice, "operate", organization)
	if err != nil || !strings.Contains(string(organizationView), instance) || strings.Contains(string(organizationView), "PRIVATE") {
		t.Fatal("organization lookup failed or leaked private fields")
	}
	view, err := s.Inspect(ctx, alice, "operate", instance)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(view), "PRIVATE") || strings.Contains(string(view), "idempotency") || strings.Contains(string(view), "oidc-owner") {
		t.Fatal("private fields leaked")
	}
	var audit int
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM support_inspections WHERE actor_owner_id=$1 AND deployment_id=$2`, alice, instance).Scan(&audit); err != nil || audit != 1 {
		t.Fatal("inspection audit absent")
	}
	history, err := s.Activity(ctx, bob, instance, false, 0)
	if err != nil || strings.Contains(string(history), alice) || !strings.Contains(string(history), op.ID) {
		t.Fatal("owner activity projection invalid")
	}
	if _, err = s.Activity(ctx, alice, instance, false, 0); err != ErrNotFound {
		t.Fatal("foreign owner read succeeded")
	}
	history, err = s.Activity(ctx, alice, instance, true, 0)
	if err != nil || !strings.Contains(string(history), "oidc-owner:"+alice) {
		t.Fatal("operator audit attribution absent")
	}
	if _, err = s.Pool.Exec(ctx, `ALTER TABLE support_inspections ADD CONSTRAINT fixture_no_audit CHECK(false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if view, err = s.Inspect(ctx, alice, "operate", instance); err == nil || len(view) != 0 {
		t.Fatal("unaudited view escaped")
	}
}

type sessionFixture struct{ owner string }

func (s sessionFixture) Authorize(w http.ResponseWriter, r *http.Request, mutation bool) (auth.Session, bool) {
	if mutation && (r.Header.Get("Origin") != "http://localhost:5173" || r.Header.Get("X-CSRF-Token") != "fixture") {
		reply(w, 403, map[string]string{"error": "CSRF rejected"})
		return auth.Session{}, false
	}
	return auth.Session{OwnerID: s.owner}, true
}
func TestSupportHTTPAuthorityAndFixedActions(t *testing.T) {
	s, actions := fixture(t)
	for _, tc := range []struct {
		name, owner, role, method, path, body string
		headers                               bool
		want                                  int
	}{
		{"operator", alice, "operate", "GET", "/api/operator/deployments/" + instance, "", false, 200},
		{"same display denied", bob, "operate", "GET", "/api/operator/deployments/" + instance, "", false, 404},
		{"other issuer denied", otherIssuer, "operate", "GET", "/api/operator/session", "", false, 404},
		{"inspect role denied action", alice, "inspect", "POST", "/api/operator/deployments/" + instance + "/lifecycle", `{}`, true, 403},
		{"CSRF", alice, "operate", "POST", "/api/operator/deployments/" + instance + "/lifecycle", `{}`, false, 403},
		{"owner role claim", alice, "operate", "POST", "/api/operator/deployments/" + instance + "/lifecycle", `{"role":"operator"}`, true, 400},
		{"invalid history", alice, "operate", "GET", "/api/operator/deployments/" + instance + "/activity?before=-1", "", false, 400},
		{"foreign owner history", alice, "operate", "GET", "/api/deployments/" + instance + "/activity", "", false, 404},
		{"owner history", bob, "operate", "GET", "/api/deployments/" + instance + "/activity", "", false, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.Settings.Identities[0].Role = tc.role
			mux := http.NewServeMux()
			s.Register(sessionFixture{tc.owner}, actions)(mux)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.headers {
				request.Header.Set("Origin", "http://localhost:5173")
				request.Header.Set("X-CSRF-Token", "fixture")
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status %d", response.Code)
			}
			if strings.Contains(response.Body.String(), "PRIVATE") {
				t.Fatal("private response leaked")
			}
		})
	}
	mux := http.NewServeMux()
	s.Register(sessionFixture{alice}, actions)(mux)
	body := `{"idempotencyKey":"77777777-7777-4777-8777-777777777777","kind":"service","state":"suspended","reason":"maintenance"}`
	var retained string
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest("POST", "/api/operator/deployments/"+instance+"/lifecycle", strings.NewReader(body))
		request.Header.Set("Origin", "http://localhost:5173")
		request.Header.Set("X-CSRF-Token", "fixture")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatal("authorized fixed action failed")
		}
		var operation lifecycle.Operation
		if json.Unmarshal(response.Body.Bytes(), &operation) != nil {
			t.Fatal("action response malformed")
		}
		if attempt == 0 {
			retained = operation.ID
		} else if retained != operation.ID {
			t.Fatal("HTTP replay created another action")
		}
	}
	var actor string
	if err := s.Pool.QueryRow(context.Background(), `SELECT actor FROM service_operations WHERE id=$1`, retained).Scan(&actor); err != nil || actor != "oidc-owner:"+alice {
		t.Fatal("operator actor was not bound to the authorized session")
	}
	request := httptest.NewRequest("GET", "/api/operator/session", nil)
	request.Header.Set("Authorization", "Bearer private")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("CLI escalation")
	}
	s.Settings = Settings{}
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/operator/session", nil))
	if response.Code != 404 {
		t.Fatal("default operator access open")
	}
}
func TestBoundedOwnerHistoryPagination(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	for i := 1; i <= 52; i++ {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO service_operations(deployment_id,idempotency_key,generation,kind,desired_state,prior_state,reason,actor,state) VALUES($1,gen_random_uuid(),$2,'service','running','running','maintenance','PRIVATE_OPERATOR_NAME','applied')`, instance, i); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.Activity(ctx, bob, instance, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items      []json.RawMessage `json:"items"`
		HasMore    bool              `json:"hasMore"`
		NextBefore int64             `json:"nextBefore"`
	}
	if json.Unmarshal(first, &page) != nil || len(page.Items) != 50 || !page.HasMore || page.NextBefore != 3 || strings.Contains(string(first), "PRIVATE") {
		t.Fatal("invalid bounded page")
	}
	second, err := s.Activity(ctx, bob, instance, false, page.NextBefore)
	if err != nil || json.Unmarshal(second, &page) != nil || len(page.Items) != 2 || page.HasMore {
		t.Fatal("history continuation failed")
	}
}
