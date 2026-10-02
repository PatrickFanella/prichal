package billing

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/auth"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/testissuer"
)

const checkoutHTTPToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func checkoutHTTPAuth(t *testing.T, s *CheckoutIntents, owner string) *auth.Service {
	t.Helper()
	issuerServer := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + issuerServer.Listener.Addr().String()
	issuer, err := testissuer.New(issuerURL, "checkout-browser", "http://127.0.0.1:5173/api/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	issuerServer.Config.Handler = issuer.Handler()
	issuerServer.Start()
	t.Cleanup(issuerServer.Close)
	owners, err := auth.New(context.Background(), s.pool, auth.Settings{Mode: "local", Issuer: issuerURL, ClientID: "checkout-browser", Origin: "http://127.0.0.1:5173"})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(checkoutHTTPToken))
	if _, err = s.pool.Exec(context.Background(), `INSERT INTO management_sessions(token_hash,owner_id,csrf_token,expires_at) VALUES($1,$2,'checkout-fixture-csrf',now()+interval '1 hour')`, digest[:], owner); err != nil {
		t.Fatal(err)
	}
	return owners
}

func checkoutHTTPRequest(t *testing.T, mux http.Handler, method, path, body string, headers map[string]string, want int) []byte {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: "pds-local-session", Value: checkoutHTTPToken})
	r.Header.Set("Origin", "http://127.0.0.1:5173")
	r.Header.Set("X-CSRF-Token", "checkout-fixture-csrf")
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s returned %d, want %d", method, w.Code, want)
	}
	for key, value := range map[string]string{"Cache-Control": "no-store", "Content-Type": "application/json", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer"} {
		if w.Header().Get(key) != value {
			t.Fatal("checkout response lost private response headers")
		}
	}
	data, err := io.ReadAll(w.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "rk_test_") || strings.Contains(string(data), "authority_key_sha256") {
		t.Fatal("private authority escaped response")
	}
	return data
}

func TestCheckoutHTTPPreparationAndExplicitOriginalDispatch(t *testing.T) {
	s, _, owner, org, calls := fixtureCheckoutIntents(t)
	// This adapter is used only for session recovery; preparation/start make no RPC.
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP preparation dispatched authority")
		w.WriteHeader(500)
	})
	var err error
	s, err = NewCheckoutIntents(true, s.pool, adapter)
	if err != nil {
		t.Fatal(err)
	}
	owners := checkoutHTTPAuth(t, s, owner)
	mux := http.NewServeMux()
	s.RegisterCheckout(owners, adapter)(mux)
	base := "/api/organizations/" + org + "/billing/checkout"
	body := `{"idempotencyKey":"` + sessionFixtureRequest.Key + `","confirmed":true}`
	var receipt CheckoutIntent
	data := checkoutHTTPRequest(t, mux, "POST", base, body, nil, 200)
	if json.Unmarshal(data, &receipt) != nil || receipt.State != "prepared" || receipt.ID == "" || receipt.CanPay {
		t.Fatal("invalid preparation receipt")
	}
	for range 2 {
		data = checkoutHTTPRequest(t, mux, "POST", base, body, nil, 200)
		var replay CheckoutIntent
		if json.Unmarshal(data, &replay) != nil || replay != receipt {
			t.Fatal("preparation retry changed intent")
		}
	}
	for _, path := range []string{base + "?key=" + receipt.Key, base + "/" + receipt.ID} {
		checkoutHTTPRequest(t, mux, "GET", path, "", nil, 200)
	}
	before := checkoutLedger(t, s)
	checkoutHTTPRequest(t, mux, "POST", base+"/"+receipt.ID+"/start", `{"confirmed":false}`, nil, 400)
	if checkoutLedger(t, s) != before {
		t.Fatal("unchecked start queued work")
	}
	for range 2 {
		checkoutHTTPRequest(t, mux, "POST", base+"/"+receipt.ID+"/start", `{"confirmed":true}`, nil, 200)
	}
	if r, e, j := checkoutCounts(t, s.pool); r != 1 || e != 2 || j != 1 || calls.Load() != 0 {
		t.Fatal("HTTP created duplicate or remote work")
	}
	checkoutHTTPRequest(t, mux, "GET", base+"/"+receipt.ID+"/session", "", nil, 409)
}

func TestCheckoutHTTPAuthenticationAndStrictInputBeforeMutation(t *testing.T) {
	s, _, owner, org, calls := fixtureCheckoutIntents(t)
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	var err error
	s, err = NewCheckoutIntents(true, s.pool, adapter)
	if err != nil {
		t.Fatal(err)
	}
	owners := checkoutHTTPAuth(t, s, owner)
	mux := http.NewServeMux()
	s.RegisterCheckout(owners, adapter)(mux)
	base := "/api/organizations/" + org + "/billing/checkout"
	valid := `{"idempotencyKey":"` + sessionFixtureRequest.Key + `","confirmed":true}`
	for _, tc := range []struct {
		method, path, body string
		headers            map[string]string
		status             int
	}{
		{"POST", base, valid, map[string]string{"Cookie": ""}, 401},
		{"POST", base, valid, map[string]string{"Origin": "http://foreign.test"}, 403},
		{"POST", base, valid, map[string]string{"X-CSRF-Token": "wrong"}, 403},
		{"POST", base, valid, map[string]string{"Authorization": ""}, 403},
		{"GET", base + "?key=" + sessionFixtureRequest.Key, "", map[string]string{"Authorization": "Bearer private-marker"}, 403},
		{"POST", base, valid, map[string]string{"Content-Type": "text/plain"}, 400},
		{"POST", base + "?override=1", valid, nil, 400},
		{"POST", base, "null", nil, 400},
		{"POST", base, "[]", nil, 400},
		{"POST", base, valid + " {}", nil, 400},
		{"POST", base, `{"idempotencyKey":"` + sessionFixtureRequest.Key + `","confirmed":true,"confirmed":false}`, nil, 400},
		{"POST", base, `{"idempotencyKey":"` + sessionFixtureRequest.Key + `","confirmed":true,"customerId":"cus_foreign"}`, nil, 400},
		{"POST", base, `{"idempotencyKey":null,"confirmed":true}`, nil, 400},
		{"POST", base, `{"idempotencyKey":"` + sessionFixtureRequest.Key + `","confirmed":null}`, nil, 400},
		{"POST", base, `{"idempotencyKey":"` + sessionFixtureRequest.Key + `"}`, nil, 400},
		{"POST", base, strings.Repeat(" ", 2049) + valid, nil, 400},
		{"GET", base + "?key=" + sessionFixtureRequest.Key + "&key=" + sessionFixtureRequest.Key, "", nil, 400},
		{"GET", base + "?key=" + sessionFixtureRequest.Key + "&customer=other", "", nil, 400},
		{"GET", base + "?key=%zz", "", nil, 400},
		{"GET", base + "?key=" + sessionFixtureRequest.Key, "{}", nil, 400},
		{"GET", base + "/" + sessionFixtureRequest.Key + "?override=1", "", nil, 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) { checkoutHTTPRequest(t, mux, tc.method, tc.path, tc.body, tc.headers, tc.status) })
	}
	if r, e, j := checkoutCounts(t, s.pool); r != 0 || e != 0 || j != 0 || calls.Load() != 0 {
		t.Fatal("rejected input created work")
	}
}

func TestCheckoutHTTPRecoveryAndForeignCustody(t *testing.T) {
	s, p, owner, adapter, calls := readyCheckoutRecovery(t, nil)
	owners := checkoutHTTPAuth(t, s, owner)
	mux := http.NewServeMux()
	s.RegisterCheckout(owners, adapter)(mux)
	path := "/api/organizations/" + p.OrganizationID + "/billing/checkout/" + p.ID
	before := checkoutLedger(t, s)
	data := checkoutHTTPRequest(t, mux, "GET", path+"/session", "", nil, 200)
	var result RecoveredCheckout
	if json.Unmarshal(data, &result) != nil || result.Status != "open" || result.URL == "" {
		t.Fatal("session recovery lost confirmed navigation")
	}
	if checkoutLedger(t, s) != before || calls.Load() != 1 {
		t.Fatal("HTTP recovery changed work")
	}
	foreign := "/api/organizations/" + sessionFixtureRequest.Key + "/billing/checkout/" + p.ID
	for _, suffix := range []string{"", "/session"} {
		checkoutHTTPRequest(t, mux, "GET", foreign+suffix, "", nil, 404)
	}
	checkoutHTTPRequest(t, mux, "POST", foreign+"/start", `{"confirmed":true}`, nil, 404)
	if checkoutLedger(t, s) != before || calls.Load() != 1 {
		t.Fatal("foreign HTTP request changed work")
	}
	if _, err := s.pool.Exec(context.Background(), `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "/session"} {
		checkoutHTTPRequest(t, mux, "GET", path+suffix, "", nil, 503)
	}
	if checkoutLedger(t, s) != before || calls.Load() != 1 {
		t.Fatal("held HTTP request changed work")
	}
}

func TestCheckoutHTTPDisabledAndNilAuthorization(t *testing.T) {
	s, p, owner, adapter, calls := readyCheckoutRecovery(t, nil)
	owners := checkoutHTTPAuth(t, s, owner)
	var disabled *CheckoutIntents
	mux := http.NewServeMux()
	disabled.RegisterCheckout(owners, nil)(mux)
	base := "/api/organizations/" + p.OrganizationID + "/billing/checkout"
	for _, request := range []struct{ method, path, body string }{
		{"POST", base, `{"confirmed":true}`},
		{"GET", base + "?key=" + p.Key, ""},
		{"GET", base + "/" + p.ID, ""},
		{"POST", base + "/" + p.ID + "/start", `{"confirmed":true}`},
		{"GET", base + "/" + p.ID + "/session", ""},
	} {
		checkoutHTTPRequest(t, mux, request.method, request.path, request.body, nil, 503)
	}
	unauth := http.NewServeMux()
	s.RegisterCheckout(nil, adapter)(unauth)
	checkoutHTTPRequest(t, unauth, "GET", base+"/"+p.ID, "", nil, 503)
	if calls.Load() != 0 {
		t.Fatal("disabled routes reached authority")
	}
}

func TestCheckoutHTTPConfiguredPlanIsReadOnlyAndCurrent(t *testing.T) {
	s, _, owner, org, calls := fixtureCheckoutIntents(t)
	adapter, _ := fixtureSessionAdapter(t, func(w http.ResponseWriter, r *http.Request) { t.Error("catalog contacted authority") })
	owners := checkoutHTTPAuth(t, s, owner)
	mux := http.NewServeMux()
	s.RegisterCheckout(owners, adapter)(mux)
	path := "/api/organizations/" + org + "/billing/checkout-plan"
	before := checkoutLedger(t, s)
	var plan CheckoutPlan
	if data := checkoutHTTPRequest(t, mux, "GET", path, "", nil, 200); json.Unmarshal(data, &plan) != nil || !plan.Fixture || !plan.CanPrepare || plan.PriceID != s.settings.PriceID {
		t.Fatal("fixed fixture catalog missing")
	}
	if checkoutLedger(t, s) != before || calls.Load() != 0 {
		t.Fatal("catalog read changed work")
	}
	if _, err := s.Prepare(context.Background(), owner, org, sessionFixtureRequest.Key, true); err != nil {
		t.Fatal(err)
	}
	before = checkoutLedger(t, s)
	if data := checkoutHTTPRequest(t, mux, "GET", path, "", nil, 200); json.Unmarshal(data, &plan) != nil || plan.CanPrepare {
		t.Fatal("catalog offered replacement for retained checkout")
	}
	if checkoutLedger(t, s) != before {
		t.Fatal("catalog availability mutated checkout")
	}
	checkoutHTTPRequest(t, mux, "GET", "/api/organizations/"+sessionFixtureRequest.Key+"/billing/checkout-plan", "", nil, 404)
	if _, err := s.pool.Exec(context.Background(), `INSERT INTO management_recovery(singleton,snapshot_sha256,state) VALUES(true,$1,'held')`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	checkoutHTTPRequest(t, mux, "GET", path, "", nil, 503)
}
