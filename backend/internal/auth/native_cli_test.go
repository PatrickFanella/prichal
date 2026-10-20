package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// nativeCLIOutput bounds captured output and exposes only the human approval code
// while the process is running. Tokens and instructions never enter test logs.
type nativeCLIOutput struct {
	mu   sync.Mutex
	data []byte
	code chan string
}

var nativeCLIUserCode = regexp.MustCompile(`enter ([A-Z2-7]{4}-[A-Z2-7]{4})`)

func (o *nativeCLIOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.data)+len(p) > 1<<20 {
		return 0, fmt.Errorf("native CLI output exceeded qualification budget")
	}
	o.data = append(o.data, p...)
	if match := nativeCLIUserCode.FindSubmatch(o.data); len(match) == 2 && o.code != nil {
		select {
		case o.code <- string(match[1]):
		default:
		}
	}
	return len(p), nil
}

func (o *nativeCLIOutput) snapshot() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Clone(o.data)
}

func TestNativeCustomerCLIAuthQualification(t *testing.T) {
	if os.Getenv("PDS_QUALIFY_CLI_AUTH") != "true" {
		t.Skip("requires explicit disposable native CLI authorization qualification")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" || os.Getenv("PDS_TEST_ADMIN_URL") == "" {
		t.Fatal("requires native Linux AMD64 and explicit local/CI test database")
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(repo, ".local/cli-build/pds-ass-linux-amd64")
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("requires an existing regular native CLI artifact")
	}
	digest := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("could not read qualification artifact")
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	before := digest(binary)
	checksumFile := filepath.Join(repo, ".local/cli-build/SHA256SUMS")
	checksums, err := os.ReadFile(checksumFile)
	if err != nil || !strings.Contains("\n"+string(checksums)+"\n", "\n"+before+"  pds-ass-linux-amd64\n") {
		t.Fatal("native CLI checksum did not match integrity metadata")
	}
	base := filepath.Join(repo, ".local/cli-native-auth")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(base, "run-")
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"result": "failed", "binarySha256": before, "qualifierSha256": digest(filepath.Join(repo, "backend/internal/auth/native_cli_test.go")), "platform": runtime.GOOS + "/" + runtime.GOARCH, "nativeDeploymentQualified": false, "publishedInstallationQualified": false, "browserUIQualified": false}
	// Registered first: the fixture's HTTP servers and disposable database clean up
	// before a successful receipt can be written.
	t.Cleanup(func() {
		if !t.Failed() {
			report["result"] = "passed"
		}
		report["completedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil || os.WriteFile(filepath.Join(work, "report.json"), append(data, '\n'), 0600) != nil {
			t.Error("could not retain native CLI authorization receipt")
		}
		t.Logf("native CLI authorization receipt: %s", filepath.Join(work, "report.json"))
	})
	f := setup(t, nil)
	tokens := []string{}
	checkOutput := func(stdout, stderr []byte) {
		t.Helper()
		for _, token := range tokens {
			if bytes.Contains(stdout, []byte(token)) || bytes.Contains(stderr, []byte(token)) {
				t.Fatal("credential appeared in captured CLI output")
			}
		}
	}
	cli := func(dir string, expected int, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append(args, "--config-dir", dir)...)
		out, stderr := &nativeCLIOutput{}, &nativeCLIOutput{}
		cmd.Stdout, cmd.Stderr = out, stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			code = -1
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			}
		}
		checkOutput(out.snapshot(), stderr.snapshot())
		if code != expected || (expected != 0 && len(out.snapshot()) != 0) {
			t.Fatal("native CLI exit/stdout contract mismatch; output withheld")
		}
		return out.snapshot()
	}
	type ownerCLI struct {
		dir, token, credentialID string
		browser                  *http.Client
		session                  Session
		organization             Organization
	}
	login := func(owner string) ownerCLI {
		t.Helper()
		b := browser(t)
		s := f.signIn(t, b, owner)
		org := decode[Organization](t, response(t, b, "POST", f.server.URL+"/api/organizations", f.server.URL, s.CSRFToken, `{"name":"Native CLI `+owner+`"}`, 201))
		dir := filepath.Join(work, owner)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "login", "--api-url", f.server.URL, "--no-browser", "--config-dir", dir)
		out, stderr := &nativeCLIOutput{}, &nativeCLIOutput{code: make(chan string, 1)}
		cmd.Stdout, cmd.Stderr = out, stderr
		if err := cmd.Start(); err != nil {
			t.Fatal("native CLI login could not start")
		}
		done := make(chan struct{})
		var waitErr error
		go func() { waitErr = cmd.Wait(); close(done) }()
		defer func() { cancel(); <-done }()
		var code string
		select {
		case code = <-stderr.code:
		case <-done:
			t.Fatal("native CLI login exited before approval; output withheld")
		case <-ctx.Done():
			t.Fatal("native CLI login timed out before approval")
		}
		payload, _ := json.Marshal(map[string]string{"userCode": code})
		closeResponse(response(t, b, "POST", f.server.URL+"/api/cli/review", f.server.URL, s.CSRFToken, string(payload), 200))
		approve(t, f, b, s, deviceGrant{UserCode: code}, true)
		<-done
		if waitErr != nil {
			t.Fatal("native CLI approved login failed; output withheld")
		}
		var credential struct {
			Token, CredentialID, APIURL string
		}
		path := filepath.Join(dir, "credentials.json")
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &credential) != nil || len(credential.Token) != 43 || credential.CredentialID == "" || credential.APIURL != f.server.URL {
			t.Fatal("invalid native CLI credential file; contents withheld")
		}
		tokens = append(tokens, credential.Token)
		checkOutput(out.snapshot(), stderr.snapshot())
		for path, mode := range map[string]os.FileMode{dir: 0700, path: 0600} {
			info, err := os.Lstat(path)
			if err != nil || info.Mode().Perm() != mode || info.Mode()&os.ModeSymlink != 0 {
				t.Fatal("unsafe native CLI credential permissions")
			}
		}
		var result struct{ SignedIn bool }
		if json.Unmarshal(out.snapshot(), &result) != nil || !result.SignedIn {
			t.Fatal("native CLI did not return signed-in JSON")
		}
		return ownerCLI{dir, credential.Token, credential.CredentialID, b, s, org}
	}
	alice, bob := login("alice"), login("bob")
	for _, owner := range []ownerCLI{alice, bob} {
		var orgs []Organization
		if json.Unmarshal(cli(owner.dir, 0, "organizations", "list"), &orgs) != nil || len(orgs) != 1 || orgs[0].ID != owner.organization.ID {
			t.Fatal("native CLI organization reads crossed owner scope")
		}
	}
	bearerResponse(t, f, alice.token, "/api/organizations", "POST", 403)
	closeResponse(response(t, bob.browser, "POST", f.server.URL+"/api/cli/credentials/"+alice.credentialID+"/revoke", f.server.URL, bob.session.CSRFToken, "", 404))
	cli(alice.dir, 0, "organizations", "list")
	closeResponse(response(t, alice.browser, "POST", f.server.URL+"/api/cli/credentials/"+alice.credentialID+"/revoke", f.server.URL, alice.session.CSRFToken, "", 200))
	bearerResponse(t, f, alice.token, "/api/organizations", "GET", 401)
	cli(alice.dir, 1, "organizations", "list")
	cli(bob.dir, 0, "organizations", "list")
	for _, owner := range []ownerCLI{alice, bob} {
		var result struct{ SignedOut bool }
		if json.Unmarshal(cli(owner.dir, 0, "logout"), &result) != nil || !result.SignedOut {
			t.Fatal("native CLI logout did not return signed-out JSON")
		}
		if _, err := os.Lstat(filepath.Join(owner.dir, "credentials.json")); !os.IsNotExist(err) {
			t.Fatal("native CLI logout retained its credential file")
		}
	}
	var active int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM cli_credentials`).Scan(&active); err != nil || active != 0 {
		t.Fatal("native CLI credentials remained active after logout")
	}
	afterChecksums, err := os.ReadFile(checksumFile)
	if err != nil || !bytes.Equal(checksums, afterChecksums) || digest(binary) != before {
		t.Fatal("native CLI artifact or integrity metadata changed")
	}
	report["deviceApprovalThroughOIDCFixture"] = true
	report["ownerScopedOrganizationReads"] = true
	report["privilegedMutationDenied"] = true
	report["foreignRevocationDenied"] = true
	report["individualRevocationEnforced"] = true
	report["bothLogoutsRemovedCredentials"] = true
	report["noActiveCLICredentials"] = true
	report["credentialsAbsentFromOutput"] = true
	report["binaryAndChecksumsUnchanged"] = true
}
