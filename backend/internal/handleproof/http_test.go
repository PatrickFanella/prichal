package handleproof

import (
	"context"
	"crypto/sha256"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPOwnerNativeCredentialAndBoundedBodyBoundary(t *testing.T) {
	store, owner, bob, deploy := newStoreFixture(t)
	ctx := context.Background()
	issuerServer := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + issuerServer.Listener.Addr().String()
	issuer, err := testissuer.New(issuerURL, "test", "http://localhost/api/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	issuerServer.Config.Handler = issuer.Handler()
	issuerServer.Start()
	defer issuerServer.Close()
	owners, err := auth.New(ctx, store.Pool, auth.Settings{Issuer: issuerURL, ClientID: "test", Origin: "http://localhost", Mode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	aliceToken, bobToken := strings.Repeat("a", 43), strings.Repeat("b", 43)
	for _, entry := range []struct{ token, owner string }{{aliceToken, owner}, {bobToken, bob}} {
		hash := sha256.Sum256([]byte(entry.token))
		if _, err = store.Pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'test-csrf',now()+interval '1 hour')`, hash[:], entry.owner); err != nil {
			t.Fatal(err)
		}
	}
	request := func(local bool, method, path, token, origin, csrf, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		store.Register(owners, local)(mux)
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
	path := "/api/deployments/" + deploy + "/handle"
	body := `{"handle":"creator.example.org"}`
	for _, entry := range []struct {
		token, origin, csrf, body string
		status                    int
	}{{"", "http://localhost", "", body, 401}, {aliceToken, "http://localhost", "wrong", body, 403}, {aliceToken, "http://foreign.invalid", "test-csrf", body, 403}, {bobToken, "http://localhost", "test-csrf", body, 404}, {aliceToken, "http://localhost", "test-csrf", `{"handle":"creator.example.org","accessJwt":"PRIVATE TOKEN"}`, 400}, {aliceToken, "http://localhost", "test-csrf", body + strings.Repeat(" ", 1100), 400}, {aliceToken, "http://localhost", "test-csrf", body, 200}} {
		w := request(true, "POST", path, entry.token, entry.origin, entry.csrf, entry.body)
		if w.Code != entry.status {
			t.Fatalf("handle boundary: %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatal("native credential echoed")
		}
	}
	if request(true, "GET", path, bobToken, "", "", "").Code != 404 {
		t.Fatal("foreign handle read")
	}
	if request(false, "POST", path, aliceToken, "http://localhost", "test-csrf", body).Code != 503 {
		t.Fatal("live proof enabled")
	}
	if request(true, "POST", path+"/check", aliceToken, "http://localhost", "test-csrf", "{}").Code != 200 {
		t.Fatal("authenticated proof failed")
	}
	if request(true, "POST", path+"/check", aliceToken, "http://localhost", "test-csrf", "{}").Code != 409 {
		t.Fatal("HTTP recheck unbounded")
	}
}
