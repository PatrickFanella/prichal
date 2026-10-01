package enrollment

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
)

func TestHTTPEnforcesOwnerOriginCSRFAndPasswordBoundary(t *testing.T) {
	f := newStoreFixture(t)
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
	owners, err := auth.New(ctx, f.pool, auth.Settings{Issuer: issuerURL, ClientID: "test", Origin: "http://localhost", Mode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	aliceToken := strings.Repeat("a", 43)
	bobToken := strings.Repeat("b", 43)
	for _, entry := range []struct{ token, owner string }{{aliceToken, f.owner}, {bobToken, f.bob}} {
		hash := sha256.Sum256([]byte(entry.token))
		if _, err = f.pool.Exec(ctx, `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'test-csrf',now()+interval '1 hour')`, hash[:], entry.owner); err != nil {
			t.Fatal(err)
		}
	}
	request := func(local bool, method, path, token, origin, csrf, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		f.store.Register(owners, local)(mux)
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
	path := "/api/deployments/" + f.deployment + "/enrollment"
	body := `{"idempotencyKey":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","recover":false}`
	for _, entry := range []struct {
		token, origin, csrf, body string
		status                    int
	}{
		{"", "http://localhost", "", body, 401},
		{aliceToken, "http://localhost", "wrong", body, 403},
		{aliceToken, "http://foreign.invalid", "test-csrf", body, 403},
		{bobToken, "http://localhost", "test-csrf", body, 404},
		{aliceToken, "http://localhost", "test-csrf", `{"idempotencyKey":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","password":"PRIVATE PDS PASSWORD"}`, 400},
		{aliceToken, "http://localhost", "test-csrf", body, 200},
	} {
		w := request(true, "POST", path, entry.token, entry.origin, entry.csrf, entry.body)
		if w.Code != entry.status {
			t.Fatalf("owner boundary: %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatal("submitted password entered response")
		}
	}
	if _, err = f.store.SetAdmission(ctx, "operator", "local_qualification", false); err != nil {
		t.Fatal(err)
	}
	policyPath := path + "/admission"
	if w := request(true, "GET", policyPath, aliceToken, "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"open":false`) || strings.Contains(w.Body.String(), "operator") {
		t.Fatal("owned policy read")
	}
	if request(true, "GET", policyPath, bobToken, "", "", "").Code != 404 {
		t.Fatal("foreign policy read")
	}
	if request(true, "POST", path, aliceToken, "http://localhost", "test-csrf", body).Code != 200 {
		t.Fatal("closed policy retained retry")
	}
	newBody := `{"idempotencyKey":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","recover":false}`
	if request(true, "POST", path, aliceToken, "http://localhost", "test-csrf", newBody).Code != 503 {
		t.Fatal("closed policy new request")
	}
	if request(false, "GET", policyPath, aliceToken, "", "", "").Code != 503 {
		t.Fatal("production policy route opened")
	}
	op, err := f.store.Latest(ctx, f.owner, f.deployment)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{path, "/api/enrollments/" + op.ID} {
		w := request(true, "GET", target, bobToken, "", "", "")
		if w.Code != 404 {
			t.Fatal("foreign enrollment read")
		}
	}
	withdrawPath := "/api/enrollments/" + op.ID + "/withdraw"
	worker := &Worker{Pool: f.pool, Cipher: f.store.Cipher, Agent: &agentFixture{}}
	if err = worker.Work(ctx, job(op.ID, 1)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		token, origin, csrf, body string
		status                    int
	}{
		{"", "http://localhost", "", "{}", 401},
		{aliceToken, "http://localhost", "wrong", "{}", 403},
		{aliceToken, "http://foreign.invalid", "test-csrf", "{}", 403},
		{bobToken, "http://localhost", "test-csrf", "{}", 404},
		{aliceToken, "http://localhost", "test-csrf", `{"action":"delete"}`, 400},
		{aliceToken, "http://localhost", "test-csrf", "{} {}", 400},
		{aliceToken, "http://localhost", "test-csrf", "{}", 200},
	} {
		w := request(true, "POST", withdrawPath, entry.token, entry.origin, entry.csrf, entry.body)
		if w.Code != entry.status {
			t.Fatalf("withdrawal boundary: %d want %d", w.Code, entry.status)
		}
		if entry.status == 200 && (!strings.Contains(w.Body.String(), `"state":"withdrawing"`) || strings.Contains(w.Body.String(), `"code":`)) {
			t.Fatal("withdrawal state or invitation disclosure")
		}
	}
	if request(false, "POST", withdrawPath, aliceToken, "http://localhost", "test-csrf", "{}").Code != 503 {
		t.Fatal("production withdrawal gate opened")
	}
	cliMux := http.NewServeMux()
	f.store.Register(owners, true)(cliMux)
	cliRequest := httptest.NewRequest("POST", withdrawPath, strings.NewReader("{}"))
	cliRequest.Header.Set("Authorization", "Bearer "+strings.Repeat("c", 43))
	cliResponse := httptest.NewRecorder()
	cliMux.ServeHTTP(cliResponse, cliRequest)
	if cliResponse.Code != 403 {
		t.Fatal("deployment-scoped CLI credential acquired withdrawal authority")
	}
	w := request(false, "POST", path, aliceToken, "http://localhost", "test-csrf", body)
	if w.Code != 503 {
		t.Fatal("live gate opened")
	}
	if _, err = f.pool.Exec(ctx, `DELETE FROM management_sessions WHERE owner_id=$1`, f.owner); err != nil {
		t.Fatal(err)
	}
	if request(true, "GET", path, aliceToken, "", "", "").Code != 401 {
		t.Fatal("revoked session retained enrollment access")
	}
}
