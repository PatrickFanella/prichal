package deployment

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/entitlement"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
)

func TestHTTPAuthorizationAndLiveGate(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	issuerServer := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + issuerServer.Listener.Addr().String()
	issuer, err := testissuer.New(issuerURL, "test", "http://localhost/api/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	issuerServer.Config.Handler = issuer.Handler()
	issuerServer.Start()
	defer issuerServer.Close()
	owners, err := auth.New(ctx, pool, auth.Settings{Issuer: issuerURL, ClientID: "test", Origin: "http://localhost", Mode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	var alice, bob, org string
	for _, entry := range []struct {
		subject string
		id      *string
	}{{"alice", &alice}, {"bob", &bob}} {
		if err = pool.QueryRow(ctx, `INSERT INTO owners(issuer,subject,display_name) VALUES('local',$1,$1) RETURNING id`, entry.subject).Scan(entry.id); err != nil {
			t.Fatal(err)
		}
	}
	if err = pool.QueryRow(ctx, `INSERT INTO organizations(id,name,owner_id) VALUES(gen_random_uuid(),'Studio',$1) RETURNING id`, alice).Scan(&org); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ token, owner string }{{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", alice}, {"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", bob}} {
		hash := sha256.Sum256([]byte(entry.token))
		if _, err = pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'test-csrf',now()+interval '1 hour')`, hash[:], entry.owner); err != nil {
			t.Fatal(err)
		}
	}
	requestBody := `{"name":"studio","region":"local","idempotencyKey":"request"}`
	request := func(local bool, method, path, token, csrf string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		store.Register(owners, local)(mux)
		r := httptest.NewRequest(method, path, strings.NewReader(requestBody))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", csrf)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "pds-local-session", Value: token})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	path := "/api/organizations/" + org + "/deployments"
	for _, entry := range []struct {
		token, csrf string
		status      int
	}{{"", "", 401}, {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "wrong", 403}, {"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "test-csrf", 404}, {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "test-csrf", 200}} {
		w := request(true, "POST", path, entry.token, entry.csrf)
		if w.Code != entry.status {
			t.Fatalf("authorization: %d %s", w.Code, w.Body.String())
		}
	}
	op, err := store.Submit(ctx, alice, org, Request{Name: "studio", Region: "local", Key: "request"})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{path, "/api/deployments/" + op.ID} {
		w := request(true, "GET", target, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "")
		if w.Code != 404 {
			t.Fatalf("foreign read: %d", w.Code)
		}
	}
	w := request(false, "POST", path, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "test-csrf")
	if w.Code != 503 || !strings.Contains(w.Body.String(), "not configured") {
		t.Fatalf("live gate: %d %s", w.Code, w.Body.String())
	}

	store.Staging = true
	requestBody = `{"kind":"agent","name":"staging-native","region":"local","idempotencyKey":"staging-request"}`
	token := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if w := request(false, "POST", path, token, "test-csrf"); w.Code != 403 {
		t.Fatalf("staging bypassed grant: %d %s", w.Code, w.Body.String())
	}
	_, err = (entitlement.Store{Pool: pool}).Submit(ctx, "test-operator", org, entitlement.Request{Key: "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", State: "active", Plan: "local-single", Hours: 24, Reason: "qualification"})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(false, "POST", path, token, "test-csrf"); w.Code != 200 {
		t.Fatalf("staging admission: %d %s", w.Code, w.Body.String())
	}
	if w := request(false, "POST", path, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "test-csrf"); w.Code != 404 {
		t.Fatalf("staging owner isolation: %d", w.Code)
	}
	requestBody = `{"kind":"test","name":"simulation","region":"local","idempotencyKey":"simulation"}`
	if w := request(false, "POST", path, token, "test-csrf"); w.Code != 400 {
		t.Fatalf("staging admitted simulation: %d", w.Code)
	}
	if err := (Router{Pool: pool, AgentOnly: true}).Step(ctx, op.ID); err == nil {
		t.Fatal("staging router ran retained simulation")
	}
}
