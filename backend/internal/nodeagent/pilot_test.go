package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPilotPolicy() PilotPolicy {
	return PilotPolicy{InstanceID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Hostname: "prichal-pds.subcult.tv", EmailFrom: "test@subcult.tv", SMTPURL: "smtps://test:disposable@mail.subcult.tv:465"}
}

func writePilotPolicy(t *testing.T, policy PilotPolicy) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.json")
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pilotRuntime(t *testing.T, directory string) *Docker {
	t.Helper()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPilotIdentitySurvivesRestartAndRefusesRemapping(t *testing.T) {
	directory := t.TempDir()
	policy := testPilotPolicy()
	d := pilotRuntime(t, directory)
	if err := d.EnablePilot(writePilotPolicy(t, policy)); err != nil {
		t.Fatal(err)
	}
	secret, err := d.credentials(policy.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	restarted := pilotRuntime(t, directory)
	if restarted.allowedInstance(policy.InstanceID) {
		t.Fatal("local runtime accepted pilot state")
	}
	if restarted.ValidateLocalState() == nil {
		t.Fatal("local startup accepted pilot state")
	}
	if err := restarted.EnablePilot(writePilotPolicy(t, policy)); err != nil {
		t.Fatal(err)
	}
	retained, err := restarted.readCredentials(policy.InstanceID)
	if err != nil || retained != secret {
		t.Fatal("pilot changed identity keys on restart")
	}
	for _, mutate := range []func(*PilotPolicy){
		func(p *PilotPolicy) { p.Hostname = "other.subcult.tv" },
		func(p *PilotPolicy) { p.InstanceID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" },
		func(p *PilotPolicy) { p.SMTPURL = "smtps://test:changed@mail.subcult.tv:465" },
	} {
		changed := policy
		mutate(&changed)
		if err := pilotRuntime(t, directory).EnablePilot(writePilotPolicy(t, changed)); err == nil {
			t.Fatal("silently remapped retained pilot policy")
		}
	}
	foreign := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if _, err := restarted.Ensure(context.Background(), foreign); err == nil {
		t.Fatal("created unapproved namespace")
	}
	if _, err := restarted.Status(context.Background(), foreign); err == nil {
		t.Fatal("inspected unapproved namespace")
	}
	if _, err := os.Stat(filepath.Join(directory, foreign+".json")); !os.IsNotExist(err) {
		t.Fatal("refusal wrote foreign credentials")
	}
}

func TestFreshPilotRefusesRetainedStorageBeforeGeneratingKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/volumes/") {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	d := pilotRuntime(t, t.TempDir())
	d.client = &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
	policy := testPilotPolicy()
	if err := d.EnablePilot(writePilotPolicy(t, policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ensure(context.Background(), policy.InstanceID); err == nil {
		t.Fatal("adopted retained storage with new keys")
	}
	if _, err := os.Stat(filepath.Join(d.directory, policy.InstanceID+".json")); !os.IsNotExist(err) {
		t.Fatal("generated keys over retained storage")
	}
}

func TestPilotPolicyRefusesUnsafeConfigurationAndLocalState(t *testing.T) {
	for _, mutate := range []func(*PilotPolicy){
		func(p *PilotPolicy) { p.Hostname = "pds.subcult.tv" },
		func(p *PilotPolicy) { p.Hostname = "127.0.0.1" },
		func(p *PilotPolicy) { p.Hostname = "a.localhost" },
		func(p *PilotPolicy) { p.Hostname = "a.subcult.tv\nrespond 200" },
		func(p *PilotPolicy) { p.EmailFrom = "Name <test@subcult.tv>" },
		func(p *PilotPolicy) { p.SMTPURL = "smtp://user:password@mail.subcult.tv:25" },
		func(p *PilotPolicy) { p.SMTPURL = "smtps://mail.subcult.tv" },
		func(p *PilotPolicy) { p.SMTPURL += "?rejectUnauthorized=false" },
	} {
		policy := testPilotPolicy()
		mutate(&policy)
		d := pilotRuntime(t, t.TempDir())
		if err := d.EnablePilot(writePilotPolicy(t, policy)); err == nil {
			t.Fatal("accepted unsafe pilot policy")
		}
		entries, err := os.ReadDir(d.directory)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid policy changed state")
		}
	}
	d := pilotRuntime(t, t.TempDir())
	if _, err := d.credentials(testPilotPolicy().InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := d.EnablePilot(writePilotPolicy(t, testPilotPolicy())); err == nil {
		t.Fatal("adopted existing local state")
	}
	path := writePilotPolicy(t, testPilotPolicy())
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := pilotRuntime(t, t.TempDir()).EnablePilot(path); err == nil {
		t.Fatal("accepted public mail credentials")
	}
}

func TestPilotHandlerExcludesManagementMutations(t *testing.T) {
	d := pilotRuntime(t, t.TempDir())
	if err := d.EnablePilot(writePilotPolicy(t, testPilotPolicy())); err != nil {
		t.Fatal(err)
	}
	handler := PilotHandler(d)
	base := "/v1/instances/" + testPilotPolicy().InstanceID
	for _, path := range []string{base + "/enrollment", base + "/service", base + "/members/invitation", base + "/account", base + "/export", "/v1/capacity", base + "?ignored=true", "/v1/instances/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"} {
		for _, method := range []string{"GET", "PUT", "POST"} {
			r := httptest.NewRequest(method, path, strings.NewReader("{}"))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("exposed %s %s: %d", method, path, w.Code)
			}
		}
	}
	// The permitted route still applies the empty-body lifecycle contract before Docker.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("PUT", base, strings.NewReader(`{"image":"foreign"}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatal("pilot accepted caller runtime configuration")
	}
}

func TestPilotUsesRealProtocolConfiguration(t *testing.T) {
	d := pilotRuntime(t, t.TempDir())
	policy := testPilotPolicy()
	if err := d.EnablePilot(writePilotPolicy(t, policy)); err != nil {
		t.Fatal(err)
	}
	environment := strings.Join(d.runtimeEnvironment(policy.InstanceID, secrets{}), "\n")
	for _, expected := range []string{"PDS_DEV_MODE=false", "PDS_HOSTNAME=" + policy.Hostname, "PDS_SERVICE_DID=did:web:" + policy.Hostname, "PDS_DID_PLC_URL=https://plc.directory", "PDS_CRAWLERS=https://bsky.network", "PDS_INVITE_REQUIRED=true", "PDS_EMAIL_SMTP_URL=" + policy.SMTPURL} {
		if !strings.Contains(environment, expected+"\n") && !strings.HasSuffix(environment, expected) {
			t.Fatalf("missing %s", expected)
		}
	}
	if strings.Contains(environment, "fixtures") || strings.Contains(environment, "PDS_DEV_MODE=true") {
		t.Fatal("pilot uses local development protocol")
	}
}
