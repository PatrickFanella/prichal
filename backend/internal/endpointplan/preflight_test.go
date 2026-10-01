package endpointplan

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
)

func TestHostnameAndRegistrableSeparation(t *testing.T) {
	for _, tc := range []struct {
		name, host, management string
		apps                   []string
		local                  bool
		want                   error
	}{
		{"public", "PDS.CREATOR.NET", "manage.platform.org", []string{"app.creator.com"}, false, nil},
		{"same-host", "creator.net", "manage.platform.org", []string{"creator.net"}, false, ErrDomainSeparation},
		{"siblings", "pds.creator.net", "manage.platform.org", []string{"chat.creator.net"}, false, ErrDomainSeparation},
		{"multi-label-siblings", "pds.creator.co.uk", "manage.platform.org", []string{"chat.creator.co.uk"}, false, ErrDomainSeparation},
		{"multi-label-distinct", "pds.creator.co.uk", "manage.platform.org", []string{"chat.another.co.uk"}, false, nil},
		{"management-site", "pds.platform.org", "manage.platform.org", []string{}, false, ErrDomainSeparation},
		{"local-test", "pds.creator.test", "localhost", []string{"app.other.test"}, true, nil},
		{"test-public", "pds.creator.test", "manage.platform.org", []string{}, false, ErrInvalid},
		{"local-siblings", "pds.creator.test", "localhost", []string{"app.creator.test"}, true, ErrDomainSeparation},
		{"ip", "127.0.0.1", "localhost", []string{}, true, ErrInvalid},
		{"url", "https://creator.net", "localhost", []string{}, true, ErrInvalid},
		{"port", "creator.net:443", "localhost", []string{}, true, ErrInvalid},
		{"dot", "creator.net.", "localhost", []string{}, true, ErrInvalid},
		{"space", " creator.net", "localhost", []string{}, true, ErrInvalid},
		{"unicode", "bücher.de", "localhost", []string{}, true, ErrInvalid},
		{"a-label", "xn--bcher-kva.de", "localhost", []string{}, true, nil},
		{"unknown", "creator.not-a-real-tld", "localhost", []string{}, true, ErrInvalid},
		{"suffix", "co.uk", "localhost", []string{}, true, ErrInvalid},
		{"reserved", "creator.invalid", "localhost", []string{}, true, ErrInvalid},
		{"omitted-apps", "creator.net", "localhost", nil, true, ErrInvalid},
		{"bad-app", "creator.net", "localhost", []string{"https://app.org"}, true, ErrInvalid},
		{"bad-management", "creator.net", "unknown.invalid", []string{}, false, ErrInvalid},
		{"large-app-list", "creator.net", "localhost", make([]string, 9), true, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Validate(tc.local, tc.management, Request{tc.host, tc.apps})
			if !errors.Is(err, tc.want) {
				t.Fatalf("validation: %v", err)
			}
			if err == nil && (result.Hostname != strings.ToLower(tc.host) || result.CanActivate || result.ChoiceSaved || result.State != "planning_only" || len(result.RemainingGates) != 5) {
				t.Fatal("planning became activation")
			}
		})
	}
	result, err := Validate(false, "manage.platform.org", Request{"creator.net", []string{"B.APP.ORG", "a.app.org", "a.app.org"}})
	if err != nil || strings.Join(result.ApplicationHosts, ",") != "a.app.org,b.app.org" {
		t.Fatal("canonical app inventory")
	}
}

const alice = "11111111-1111-4111-8111-111111111111"
const bob = "22222222-2222-4222-8222-222222222222"
const org = "33333333-3333-4333-8333-333333333333"

func fixture(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pool := testdb.New(t)
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO owners(id,issuer,subject,display_name) VALUES($1,'local','alice','PRIVATE_OWNER'),($2,'local','bob','PRIVATE_OWNER');`, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES($1,$2,'endpoint planning')`, org, alice); err != nil {
		t.Fatal(err)
	}
	return &Store{pool, true, "localhost"}
}
func TestCurrentOwnerPreflightPreservesExistingNativeEndpoint(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	request := Request{"creator.test", []string{"app.other.test"}}
	if _, err := s.Preflight(ctx, bob, org, request); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign planning")
	}
	before, err := s.Preflight(ctx, alice, org, request)
	if err != nil || before.State != "planning_only" {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, `INSERT INTO deployments(id,organization_id,name,region,idempotency_key,kind,state,managed_hostname,endpoint_url) VALUES('44444444-4444-4444-8444-444444444444',$1,'retained','local','retained','agent','ready','PRIVATE_HOST.localhost','https://PRIVATE_HOST.localhost')`, org); err != nil {
		t.Fatal(err)
	}
	result, err := s.Preflight(ctx, alice, org, request)
	if err != nil || result.State != "existing_native_requires_review" || result.CanActivate || result.ChoiceSaved {
		t.Fatal("existing host change permitted")
	}
	var endpoint string
	var audits int
	if err = s.Pool.QueryRow(ctx, `SELECT endpoint_url FROM deployments WHERE organization_id=$1`, org).Scan(&endpoint); err != nil || endpoint != "https://PRIVATE_HOST.localhost" {
		t.Fatal("endpoint changed")
	}
	if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM organization_access_events`).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("audit mutation")
	}
	raw, err := json.Marshal(result)
	if err != nil || strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("private projection")
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE organizations SET owner_id=$1 WHERE id=$2`, bob, org); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Preflight(ctx, alice, org, request); !errors.Is(err, ErrNotFound) {
		t.Fatal("former owner planning")
	}
}

type sessions struct {
	owner          string
	calledMutation bool
}

func (s *sessions) Authorize(w http.ResponseWriter, r *http.Request, mutation bool) (auth.Session, bool) {
	s.calledMutation = mutation
	if r.Header.Get("Origin") != "http://localhost" || r.Header.Get("X-CSRF-Token") != "fixture-csrf" {
		reply(w, 403, map[string]string{"error": "fixture denied"})
		return auth.Session{}, false
	}
	return auth.Session{OwnerID: s.owner}, true
}
func TestHTTPBoundaryAndNoActivation(t *testing.T) {
	s := fixture(t)
	authn := &sessions{owner: alice}
	mux := http.NewServeMux()
	s.Register(authn)(mux)
	path := "/api/organizations/" + org + "/endpoint-preflight"
	for _, tc := range []struct {
		body, header string
		want         int
	}{
		{`{"hostname":"creator.test","applicationHosts":[]}`, "", 200},
		{`{"hostname":"creator.test","applicationHosts":[],"password":"PRIVATE"}`, "", 400},
		{`{"hostname":"creator.test","applicationHosts":[]} {}`, "", 400},
		{strings.Repeat("x", 4097), "", 400},
		{`{"hostname":"creator.test","applicationHosts":[]}`, "Bearer private", 403},
	} {
		r := httptest.NewRequest("POST", path, strings.NewReader(tc.body))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "fixture-csrf")
		if tc.header != "" {
			r.Header.Set("Authorization", tc.header)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatal("HTTP boundary")
		}
	}
	if !authn.calledMutation {
		t.Fatal("browser intent lost CSRF boundary")
	}
	s.Local = false
	r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "fixture-csrf")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("production gate")
	}
}
