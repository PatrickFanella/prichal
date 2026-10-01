package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type deviceGrant struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	Interval        int    `json:"interval"`
}
type cliToken struct {
	AccessToken  string `json:"accessToken"`
	CredentialID string `json:"credentialId"`
}

func deviceStart(t *testing.T, f *fixture) deviceGrant {
	t.Helper()
	return decode[deviceGrant](t, response(t, browser(t), "POST", f.server.URL+"/api/cli/device", "", "", `{"clientId":"pds-ass-cli"}`, 200))
}
func devicePoll(t *testing.T, f *fixture, g deviceGrant, status int) *http.Response {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"clientId": cliClient, "deviceCode": g.DeviceCode})
	return response(t, browser(t), "POST", f.server.URL+"/api/cli/token", "", "", string(b), status)
}
func approve(t *testing.T, f *fixture, b *http.Client, s Session, g deviceGrant, yes bool) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"userCode": g.UserCode, "approved": yes})
	closeResponse(response(t, b, "POST", f.server.URL+"/api/cli/approve", f.server.URL, s.CSRFToken, string(payload), 200))
}
func resetPoll(t *testing.T, f *fixture, g deviceGrant) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE cli_device_grants SET last_poll_at=now()-interval '20 minutes' WHERE device_hash=$1`, hash(g.DeviceCode)); err != nil {
		t.Fatal(err)
	}
}
func bearerResponse(t *testing.T, f *fixture, token, path, method string, status int, headers ...string) {
	t.Helper()
	req, _ := http.NewRequest(method, f.server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	for i := 0; i < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := browser(t).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		t.Fatalf("%s %s status %d, want %d", method, path, res.StatusCode, status)
	}
}
func TestDeviceGrantConsumptionScopeAndRevocation(t *testing.T) {
	f := setup(t, nil)
	b := browser(t)
	owner := f.signIn(t, b, "alice")
	g := deviceStart(t, f)
	if !humanCode.MatchString(g.UserCode) || len(g.DeviceCode) != 43 || g.VerificationURI != f.server.URL+"/device" || g.Interval != 5 {
		t.Fatal("unsafe device metadata")
	}
	if result := decode[map[string]any](t, devicePoll(t, f, g, 400)); result["error"] != "authorization_pending" {
		t.Fatal("pending grant issued token")
	}
	if result := decode[map[string]any](t, devicePoll(t, f, g, 400)); result["error"] != "slow_down" || result["interval"] != float64(10) {
		t.Fatal("poll limit not increased")
	}
	payload := `{"userCode":"` + g.UserCode + `","approved":true}`
	closeResponse(response(t, b, "POST", f.server.URL+"/api/cli/approve", "http://attacker.test", owner.CSRFToken, payload, 403))
	approve(t, f, b, owner, g, true)
	resetPoll(t, f, g)
	token := decode[cliToken](t, devicePoll(t, f, g, 200))
	if len(token.AccessToken) != 43 {
		t.Fatal("missing CLI credential")
	}
	closeResponse(devicePoll(t, f, g, 400))
	bearerResponse(t, f, token.AccessToken, "/api/organizations", "GET", 200)
	for _, path := range []string{"/api/session", "/api/cli/credentials"} {
		bearerResponse(t, f, token.AccessToken, path, "GET", 403)
	}
	bearerResponse(t, f, token.AccessToken, "/api/organizations", "POST", 403)
	bearerResponse(t, f, token.AccessToken, "/api/auth/revoke-all", "POST", 403)
	bearerResponse(t, f, token.AccessToken, "/api/organizations", "GET", 401, "Origin", f.server.URL)
	bearerResponse(t, f, token.AccessToken, "/api/organizations", "GET", 401, "Cookie", "pds-session=captured")
	bob := browser(t)
	bs := f.signIn(t, bob, "bob")
	closeResponse(response(t, bob, "POST", f.server.URL+"/api/cli/credentials/"+token.CredentialID+"/revoke", f.server.URL, bs.CSRFToken, "", 404))
	closeResponse(response(t, b, "POST", f.server.URL+"/api/cli/credentials/"+token.CredentialID+"/revoke", f.server.URL, owner.CSRFToken, "", 200))
	bearerResponse(t, f, token.AccessToken, "/api/organizations", "GET", 401)
	var plaintext int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM cli_device_grants WHERE device_hash=$1 OR user_hash=$2`, []byte(g.DeviceCode), []byte(g.UserCode)).Scan(&plaintext); err != nil || plaintext != 0 {
		t.Fatal("plaintext code stored")
	}
}
func TestDeviceExpiryDenialAndAllDevices(t *testing.T) {
	f := setup(t, nil)
	b := browser(t)
	owner := f.signIn(t, b, "alice")
	g := deviceStart(t, f)
	approve(t, f, b, owner, g, false)
	if v := decode[map[string]any](t, devicePoll(t, f, g, 400)); v["error"] != "access_denied" {
		t.Fatal("denial ignored")
	}
	expired := deviceStart(t, f)
	if _, err := f.pool.Exec(context.Background(), `UPDATE cli_device_grants SET expires_at=now()-interval '1 second' WHERE device_hash=$1`, hash(expired.DeviceCode)); err != nil {
		t.Fatal(err)
	}
	if v := decode[map[string]any](t, devicePoll(t, f, expired, 400)); v["error"] != "expired_token" {
		t.Fatal("expiry ignored")
	}
	g = deviceStart(t, f)
	approve(t, f, b, owner, g, true)
	tok := decode[cliToken](t, devicePoll(t, f, g, 200))
	waiting := deviceStart(t, f)
	approve(t, f, b, owner, waiting, true)
	closeResponse(response(t, b, "POST", f.server.URL+"/api/auth/revoke-all", f.server.URL, owner.CSRFToken, "", 200))
	bearerResponse(t, f, tok.AccessToken, "/api/organizations", "GET", 401)
	closeResponse(devicePoll(t, f, waiting, 400))
}
func TestDeviceLimitsAndStrictInputs(t *testing.T) {
	f := setup(t, nil)
	for _, body := range []string{`{"clientId":"other"}`, `{"clientId":"pds-ass-cli","scope":"admin"}`, `{"clientId":"pds-ass-cli"} {}`} {
		closeResponse(response(t, browser(t), "POST", f.server.URL+"/api/cli/device", "", "", body, 400))
	}
	for i := 0; i < 12; i++ {
		deviceStart(t, f)
	}
	closeResponse(response(t, browser(t), "POST", f.server.URL+"/api/cli/device", "", "", `{"clientId":"pds-ass-cli"}`, 429))
	b := browser(t)
	s := f.signIn(t, b, "alice")
	for i := 0; i < 10; i++ {
		closeResponse(response(t, b, "POST", f.server.URL+"/api/cli/review", f.server.URL, s.CSRFToken, `{"userCode":"AAAA-AAAA"}`, 400))
	}
	closeResponse(response(t, b, "POST", f.server.URL+"/api/cli/review", f.server.URL, s.CSRFToken, `{"userCode":"AAAA-AAAA"}`, 429))
	for _, path := range []string{"/api/organizations/AAA", "/api/deployments/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa/credentials", "/api/organizations/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa/enrollment"} {
		if cliRoute("GET", path) {
			t.Fatal("scope permits unrelated route")
		}
	}
	if cliRoute("PATCH", "/api/organizations/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa") || strings.Contains(cliClient, "admin") {
		t.Fatal("privileged scope")
	}
}

func TestConcurrentDeviceConsumptionIssuesOneCredential(t *testing.T) {
	f := setup(t, nil)
	b := browser(t)
	owner := f.signIn(t, b, "alice")
	g := deviceStart(t, f)
	approve(t, f, b, owner, g, true)
	body, _ := json.Marshal(map[string]string{"clientId": cliClient, "deviceCode": g.DeviceCode})
	results := make(chan int, 8)
	for i := 0; i < 8; i++ {
		go func() {
			req, _ := http.NewRequest("POST", f.server.URL+"/api/cli/token", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- 0
				return
			}
			res.Body.Close()
			results <- res.StatusCode
		}()
	}
	success := 0
	for i := 0; i < 8; i++ {
		status := <-results
		if status == 200 {
			success++
		} else if status != 400 {
			t.Fatalf("unexpected consumption status %d", status)
		}
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM cli_credentials WHERE owner_id=$1`, owner.OwnerID).Scan(&count); err != nil || count != 1 || success != 1 {
		t.Fatal("device consumption was not atomic")
	}
}
