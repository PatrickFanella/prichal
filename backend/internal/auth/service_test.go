package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/app"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testdb"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	server       *httptest.Server
	issuerServer *httptest.Server
	issuer       *testissuer.Issuer
	service      *Service
	pool         *pgxpool.Pool
}

func setup(t *testing.T, pool *pgxpool.Pool, secrets ...string) *fixture {
	t.Helper()
	if pool == nil {
		pool = testdb.New(t)
	}
	if err := migrate.Apply(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	issuerServer := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + issuerServer.Listener.Addr().String()
	issuer, err := testissuer.New(issuerURL, "test-management", origin+"/api/auth/callback")
	if err != nil {
		t.Fatal(err)
	}

	secret := ""
	if len(secrets) > 0 {
		secret = secrets[0]
	}
	issuer.ClientSecret = secret
	issuerServer.Config.Handler = issuer.Handler()
	issuerServer.Start()
	t.Cleanup(issuerServer.Close)
	service, err := New(context.Background(), pool, Settings{Mode: "local", Issuer: issuerURL, ClientID: "test-management", ClientSecret: secret, Origin: origin})
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = app.Handler(migrate.Probe{Pool: pool}, service.Register)
	server.Start()
	t.Cleanup(server.Close)
	return &fixture{server: server, issuerServer: issuerServer, issuer: issuer, service: service, pool: pool}
}
func browser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
}
func response(t *testing.T, client *http.Client, method, target, origin, csrf, body string, status int) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != status {
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("%s %s returned %d, want %d: %s", method, req.URL.Path, res.StatusCode, status, data)
	}
	return res
}
func closeResponse(res *http.Response) { _, _ = io.Copy(io.Discard, res.Body); res.Body.Close() }
func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer res.Body.Close()
	var value T
	if err := json.NewDecoder(res.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

var requestID = regexp.MustCompile(`name="request" value="([^"]+)"`)

func (f *fixture) begin(t *testing.T, client *http.Client, owner string) string {
	t.Helper()
	res := response(t, client, "GET", f.server.URL+"/api/auth/login", "", "", "", 303)
	authorize := res.Header.Get("Location")
	closeResponse(res)
	res = response(t, client, "GET", authorize, "", "", "", 200)
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	match := requestID.FindSubmatch(data)
	if len(match) != 2 {
		t.Fatal("fixture authorization form missing")
	}
	form := url.Values{"request": {string(match[1])}, "owner": {owner}}
	req, _ := http.NewRequest("POST", f.issuerServer.URL+"/approve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", f.issuerServer.URL)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 303 {
		t.Fatalf("fixture approval: %d", res.StatusCode)
	}
	return res.Header.Get("Location")
}
func (f *fixture) signIn(t *testing.T, client *http.Client, owner string) Session {
	t.Helper()
	callback := f.begin(t, client, owner)
	res := response(t, client, "GET", callback, "", "", "", 303)
	if res.Header.Get("Location") != f.server.URL+"/" {
		t.Fatal("callback destination changed")
	}
	var sessionCookie *http.Cookie
	for _, cookie := range res.Cookies() {
		if cookie.Name == f.service.settings.cookie("session") {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.Path != "/" || sessionCookie.Domain != "" {
		t.Fatal("unsafe session cookie")
	}
	closeResponse(res)
	return decode[Session](t, response(t, client, "GET", f.server.URL+"/api/session", "", "", "", 200))
}
func TestOIDCValidation(t *testing.T) {
	for _, fault := range []string{"issuer", "audience", "nonce", "expired", "signature"} {
		t.Run(fault, func(t *testing.T) {
			f := setup(t, nil)
			client := browser(t)
			f.issuer.SetFault(fault)
			callback := f.begin(t, client, "alice")
			res := response(t, client, "GET", callback, "", "", "", 400)
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if strings.Contains(string(body), "id_token") || strings.Contains(string(body), "fixture.test") {
				t.Fatal("token claims leaked in callback error")
			}
			closeResponse(response(t, client, "GET", callback, "", "", "", 400))
			closeResponse(response(t, client, "GET", f.server.URL+"/api/session", "", "", "", 401))
		})
	}
}
func TestBrowserBindingStateReplayAndPersistence(t *testing.T) {
	f := setup(t, nil)
	client := browser(t)
	callback := f.begin(t, client, "alice")
	closeResponse(response(t, browser(t), "GET", callback, "", "", "", 400))
	parsed, _ := url.Parse(callback)
	q := parsed.Query()
	q.Set("state", randomToken())
	parsed.RawQuery = q.Encode()
	closeResponse(response(t, client, "GET", parsed.String(), "", "", "", 400))
	closeResponse(response(t, client, "GET", callback, "", "", "", 303))
	closeResponse(response(t, client, "GET", callback, "", "", "", 400))
	first := decode[Session](t, response(t, client, "GET", f.server.URL+"/api/session", "", "", "", 200))
	if first.DisplayName != "Alice" {
		t.Fatal("wrong owner")
	}
	// A reconstructed HTTP service reads the same PostgreSQL-backed session.
	restarted := httptest.NewUnstartedServer(nil)
	origin := "http://" + restarted.Listener.Addr().String()
	settings := f.service.settings
	settings.Origin = origin
	service, err := New(context.Background(), f.pool, settings)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Config.Handler = app.Handler(migrate.Probe{Pool: f.pool}, service.Register)
	restarted.Start()
	defer restarted.Close()
	second := decode[Session](t, response(t, client, "GET", restarted.URL+"/api/session", "", "", "", 200))
	if second.OwnerID != first.OwnerID {
		t.Fatal("session did not survive API reconstruction")
	}
}
func TestOwnersAreIsolatedAndMutationsRequireCSRF(t *testing.T) {
	f := setup(t, nil)
	alice, bob := browser(t), browser(t)
	a := f.signIn(t, alice, "alice")
	b := f.signIn(t, bob, "bob")
	if a.OwnerID == b.OwnerID {
		t.Fatal("owners with matching email merged")
	}
	endpoint := f.server.URL + "/api/organizations"
	closeResponse(response(t, browser(t), "GET", endpoint, "", "", "", 401))
	closeResponse(response(t, alice, "POST", endpoint, f.server.URL, "", `{"name":"Alice house"}`, 403))
	closeResponse(response(t, alice, "POST", endpoint, "http://attacker.test", a.CSRFToken, `{"name":"Alice house"}`, 403))
	closeResponse(response(t, alice, "POST", endpoint, f.server.URL, a.CSRFToken, `{"name":" "}`, 400))
	organization := decode[Organization](t, response(t, alice, "POST", endpoint, f.server.URL, a.CSRFToken, `{"name":"Alice house"}`, 201))
	if items := decode[[]Organization](t, response(t, bob, "GET", endpoint, "", "", "", 200)); len(items) != 0 {
		t.Fatal("Bob enumerated Alice organization")
	}
	target := endpoint + "/" + organization.ID
	closeResponse(response(t, bob, "GET", target, "", "", "", 404))
	closeResponse(response(t, bob, "PATCH", target, f.server.URL, b.CSRFToken, `{"name":"Stolen"}`, 404))
	renamed := decode[Organization](t, response(t, alice, "PATCH", target, f.server.URL, a.CSRFToken, `{"name":"Alice studio"}`, 200))
	if renamed.Name != "Alice studio" {
		t.Fatal("owner rename failed")
	}
	otherIssuer := setup(t, f.pool)
	otherAlice := otherIssuer.signIn(t, browser(t), "alice")
	if otherAlice.OwnerID == a.OwnerID {
		t.Fatal("same subject from different issuer merged")
	}
	closeResponse(response(t, alice, "POST", f.server.URL+"/api/auth/logout", f.server.URL, a.CSRFToken, "", 200))
	closeResponse(response(t, alice, "GET", target, "", "", "", 401))
}
func TestExpiryAndAllSessionRevocation(t *testing.T) {
	f := setup(t, nil)
	one, two := browser(t), browser(t)
	a := f.signIn(t, one, "alice")
	f.signIn(t, two, "alice")
	closeResponse(response(t, one, "POST", f.server.URL+"/api/auth/revoke-all", f.server.URL, a.CSRFToken, "", 200))
	closeResponse(response(t, two, "GET", f.server.URL+"/api/session", "", "", "", 401))
	expired := f.signIn(t, one, "alice")
	if _, err := f.pool.Exec(context.Background(), `UPDATE management_sessions SET expires_at=now()-interval '1 second' WHERE owner_id=$1`, expired.OwnerID); err != nil {
		t.Fatal(err)
	}
	closeResponse(response(t, one, "GET", f.server.URL+"/api/session", "", "", "", 401))
}
func TestProductionCookie(t *testing.T) {
	s := Service{settings: Settings{Mode: "production"}}
	w := httptest.NewRecorder()
	s.cookie(w, "session", "opaque-value", 3600)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Name != "__Host-pds-session" || cookies[0].Domain != "" || cookies[0].Path != "/" {
		t.Fatal("production cookie must be secure, HttpOnly and host-only")
	}
}

func TestLoginExpiryAndPKCE(t *testing.T) {
	for _, failure := range []string{"expired state", "wrong PKCE"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t, nil)
			client := browser(t)
			callback := f.begin(t, client, "alice")
			parsed, _ := url.Parse(callback)
			stateHash := hash(parsed.Query().Get("state"))
			query := `UPDATE login_attempts SET expires_at=now()-interval '1 second' WHERE state_hash=$1`
			if failure == "wrong PKCE" {
				query = `UPDATE login_attempts SET pkce_verifier='not-the-original-verifier' WHERE state_hash=$1`
			}
			if _, err := f.pool.Exec(context.Background(), query, stateHash); err != nil {
				t.Fatal(err)
			}
			closeResponse(response(t, client, "GET", callback, "", "", "", 400))
			closeResponse(response(t, client, "GET", f.server.URL+"/api/session", "", "", "", 401))
		})
	}
}

func TestLogoutRejectsCapturedSessionCookie(t *testing.T) {
	f := setup(t, nil)
	client := browser(t)
	session := f.signIn(t, client, "alice")
	origin, _ := url.Parse(f.server.URL)
	captured := client.Jar.Cookies(origin)
	closeResponse(response(t, client, "POST", f.server.URL+"/api/auth/logout", f.server.URL, session.CSRFToken, "", 200))
	replay := browser(t)
	replay.Jar.SetCookies(origin, captured)
	closeResponse(response(t, replay, "GET", f.server.URL+"/api/session", "", "", "", 401))
}

func TestConfidentialClientCodeFlow(t *testing.T) {
	f := setup(t, nil, "confidential-fixture-secret-only")
	session := f.signIn(t, browser(t), "alice")
	if session.DisplayName != "Alice" {
		t.Fatal("confidential client flow failed")
	}
}
