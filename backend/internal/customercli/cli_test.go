package customercli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const orgID = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const deploymentID = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"
const keyID = "cccccccc-cccc-4ccc-cccc-cccccccccccc"
const tokenFixture = "0123456789012345678901234567890123456789012"

func cliFixture(t *testing.T, handler http.HandlerFunc) (string, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	dir := filepath.Join(t.TempDir(), "cli")
	if err := privateDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(filepath.Join(dir, "credentials.json"), config{srv.URL, tokenFixture, orgID, time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return dir, srv.URL
}
func runCLI(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(""), &out, &err)
	return code, out.String(), err.String()
}
func TestHelpAndCompletionWithoutConfiguration(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}, {"deploy", "--help"}, {"login", "--help"}, {"completion", "bash"}, {"completion", "zsh"}, {"completion", "fish"}, {"completion", "--help"}} {
		code, out, err := runCLI(t, args)
		if code != 0 || out == "" || err != "" {
			t.Fatalf("offline help failed: %v %d %s", args, code, err)
		}
	}
}
func TestEndpointAndPrivateFiles(t *testing.T) {
	for _, value := range []string{"http://remote.test", "https://user:secret@host.test", "https://host.test/path", "https://host.test?token=x", "https://host.test#x", "file:///tmp/x"} {
		if ValidateAPI(value) == nil {
			t.Fatal("unsafe API accepted")
		}
	}
	for _, value := range []string{"https://host.test", "http://localhost:5173", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if ValidateAPI(value) != nil {
			t.Fatal("valid API rejected")
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if readPrivate(path, &config{}) == nil {
		t.Fatal("public credential file accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if readPrivate(link, &config{}) == nil {
		t.Fatal("symlink credential accepted")
	}
}
func TestStableDeploymentReplayAndSecretFreeOutput(t *testing.T) {
	calls := 0
	dir, _ := cliFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tokenFixture || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
			t.Error("unsafe authentication transport")
		}
		if r.URL.Path != "/api/organizations/"+orgID+"/deployments" {
			t.Error("wrong scoped path")
		}
		var payload request
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Key != keyID || payload.Name != "local-cli" {
			t.Error("request identity changed")
		}
		calls++
		json.NewEncoder(w).Encode(map[string]string{"id": deploymentID, "organizationId": orgID, "kind": "test", "name": "local-cli", "state": "ready", "accessToken": tokenFixture})
	})
	args := []string{"deploy", "--config-dir", dir, "--organization", orgID, "--name", "local-cli", "--kind", "test", "--idempotency-key", keyID}
	for i := 0; i < 2; i++ {
		code, out, err := runCLI(t, args)
		if code != 0 || !strings.Contains(out, deploymentID) || strings.Contains(out, tokenFixture) || strings.Contains(err, tokenFixture) {
			t.Fatalf("deployment output failed: %d %s", code, err)
		}
	}
	changed := append([]string{}, args...)
	changed[7] = "other-name"
	code, _, _ := runCLI(t, changed)
	if code != 2 || calls != 2 {
		t.Fatal("changed request reached API")
	}
	var saved savedRequest
	if err := readPrivate(filepath.Join(dir, "request-"+keyID+".json"), &saved); err != nil || saved.Request.Key != keyID {
		t.Fatal("request not persisted")
	}
}
func TestPendingFailureAndRevokedLogout(t *testing.T) {
	state := "queued"
	dir, _ := cliFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cli/logout" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(operation{ID: deploymentID, OrganizationID: orgID, Name: "pending", State: state})
	})
	args := []string{"status", "--deployment", deploymentID, "--config-dir", dir}
	code, _, _ := runCLI(t, args)
	if code != 3 {
		t.Fatal("pending exit code")
	}
	state = "failed"
	code, _, _ = runCLI(t, args)
	if code != 4 {
		t.Fatal("failed exit code")
	}
	code, _, _ = runCLI(t, []string{"logout", "--config-dir", dir})
	if code != 0 {
		t.Fatal("revoked credential logout failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatal("revoked credential file retained")
	}
}
func TestInterruptedWaitAndNoninteractiveFlags(t *testing.T) {
	dir, _ := cliFixture(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(operation{ID: deploymentID, State: "queued"})
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, err bytes.Buffer
	if Run(ctx, []string{"status", "--deployment", deploymentID, "--wait", "--config-dir", dir}, strings.NewReader(""), &out, &err) != 130 {
		t.Fatal("interrupt exit code")
	}
	code, _, _ := runCLI(t, []string{"deploy", "--config-dir", dir})
	if code != 2 {
		t.Fatal("missing flags prompted in noninteractive mode")
	}
}
func TestRedirectDoesNotForwardCredential(t *testing.T) {
	called := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer other.Close()
	dir, _ := cliFixture(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, 307) })
	code, _, _ := runCLI(t, []string{"organizations", "list", "--config-dir", dir})
	if code != 1 || called {
		t.Fatal("credential redirect followed")
	}
}
func TestLoginErrorReturnsFailureWithoutTokenOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429); w.Write([]byte(tokenFixture)) }))
	defer srv.Close()
	code, out, err := runCLI(t, []string{"login", "--api-url", srv.URL, "--no-browser", "--config-dir", filepath.Join(t.TempDir(), "cli")})
	if code != 1 || strings.Contains(out+err, tokenFixture) {
		t.Fatal("login failure leaked secret or returned success")
	}
}
