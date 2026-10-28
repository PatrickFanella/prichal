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

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managednamespace"
)

const managedFixtureID = "2395105c-9958-47b5-ae8b-5b0424cbfc4f"

func managedFixture(t *testing.T) *Docker {
	t.Helper()
	d := pilotRuntime(t, t.TempDir())
	if err := d.EnableManagedEndpoints(); err != nil {
		t.Fatal(err)
	}
	return d
}

func retainManagedReceipt(t *testing.T, d *Docker, id string) string {
	t.Helper()
	host, err := managednamespace.Endpoint(id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(managedIdentity{Version: 1, InstanceID: id, Hostname: host})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d.directory, id+managedIdentitySuffix)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManagedReceiptRetainsIdentityBeforeCredentialsAndRefusesPrivateRestart(t *testing.T) {
	d := managedFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("failing preflight mutated Docker")
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	d.client = &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
	// Capacity is unavailable after the fresh-namespace fence: retain identity, but generate no keys.
	if _, err := d.EnsureManaged(context.Background(), managedFixtureID); err == nil {
		t.Fatal("missing capacity accepted")
	}
	path := filepath.Join(d.directory, managedFixtureID+managedIdentitySuffix)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d.directory, managedFixtureID+".json")); !os.IsNotExist(err) {
		t.Fatal("keys created before qualified capacity")
	}
	restarted := pilotRuntime(t, d.directory)
	if restarted.ValidateLocalState() == nil {
		t.Fatal("private startup accepted managed state")
	}
	if _, err := restarted.Ensure(context.Background(), managedFixtureID); err == nil {
		t.Fatal("private reconciliation adopted managed state")
	}
	if err := restarted.EnableManagedEndpoints(); err != nil {
		t.Fatal(err)
	}
	restarted.client = d.client
	if _, err := restarted.EnsureManaged(context.Background(), managedFixtureID); err == nil {
		t.Fatal("missing capacity accepted on replay")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("restart changed retained identity", err)
	}
	value, err := restarted.readManagedIdentity(managedFixtureID)
	if err != nil || value.Hostname != "pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv" {
		t.Fatal(value, err)
	}
}

func TestManagedIdentityRefusesRetainedPrivateStateAndForeignResources(t *testing.T) {
	for _, kind := range []string{"credentials", "tls", "volume", "container", "network"} {
		t.Run(kind, func(t *testing.T) {
			d := managedFixture(t)
			if kind == "credentials" {
				if _, err := d.credentials(managedFixtureID); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "tls" {
				if _, err := d.localCertificate(managedFixtureID); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("adoption refusal mutated Docker")
				}
				if kind == "volume" && strings.Contains(r.URL.Path, "/volumes/") || kind == "container" && strings.Contains(r.URL.Path, "/containers/") || kind == "network" && strings.Contains(r.URL.Path, "/networks/") {
					w.WriteHeader(200)
					return
				}
				w.WriteHeader(404)
			}))
			defer server.Close()
			d.client = &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
			if _, err := d.EnsureManaged(context.Background(), managedFixtureID); err == nil {
				t.Fatal("adopted retained private namespace")
			}
			if d.ManagedIdentityRetained(managedFixtureID) {
				t.Fatal("refused adoption wrote public identity")
			}
		})
	}
}

func TestManagedReceiptTamperingAndMutationsFailClosed(t *testing.T) {
	for _, kind := range []string{"hostname", "version", "permissions", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			d := managedFixture(t)
			path := retainManagedReceipt(t, d, managedFixtureID)
			switch kind {
			case "hostname":
				data, _ := os.ReadFile(path)
				if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "pds-2395105c995847b5ae8b5b0424cbfc4f.subcult.tv", "other.subcult.tv")), 0600); err != nil {
					t.Fatal(err)
				}
			case "version":
				if err := os.WriteFile(path, []byte(`{"version":2}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".retained"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".retained", path); err != nil {
					t.Fatal(err)
				}
			}
			if pilotRuntime(t, d.directory).EnableManagedEndpoints() == nil {
				t.Fatal("startup accepted tampered receipt")
			}
			if _, err := d.EnsureManaged(context.Background(), managedFixtureID); err == nil {
				t.Fatal("managed reconciliation accepted tampered receipt")
			}
			if _, err := d.Ensure(context.Background(), managedFixtureID); err == nil {
				t.Fatal("tampered receipt fell back to private identity")
			}
		})
	}
	d := managedFixture(t)
	// An as-yet unreserved ID cannot race receipt creation to obtain a mutation.
	w := httptest.NewRecorder()
	Handler(d).ServeHTTP(w, httptest.NewRequest("POST", "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa/service", strings.NewReader("{}")))
	if w.Code != 404 {
		t.Fatal("managed opt-in exposed unsupported pre-receipt mutation", w.Code)
	}
	retainManagedReceipt(t, d, managedFixtureID)
	base := "/v1/instances/" + managedFixtureID
	for _, suffix := range []string{"", "/enrollment", "/enrollment/withdraw", "/service", "/account", "/members/invitation", "/members/exit", "/export"} {
		for _, method := range []string{"POST", "PUT"} {
			w := httptest.NewRecorder()
			Handler(d).ServeHTTP(w, httptest.NewRequest(method, base+suffix, strings.NewReader("{}")))
			if w.Code != 404 {
				t.Fatalf("managed identity exposed unsupported %s %s: %d", method, suffix, w.Code)
			}
		}
	}
	for _, path := range []string{"?hostname=foreign.subcult.tv", "/extra"} {
		w := httptest.NewRecorder()
		Handler(d).ServeHTTP(w, httptest.NewRequest("PUT", "/v1/managed-instances/"+managedFixtureID+path, strings.NewReader("{}")))
		if w.Code != 404 {
			t.Fatal("managed route accepted extra authority", w.Code)
		}
	}
	w = httptest.NewRecorder()
	Handler(d).ServeHTTP(w, httptest.NewRequest("PUT", "/v1/managed-instances/"+managedFixtureID, strings.NewReader(`{"hostname":"foreign.subcult.tv"}`)))
	if w.Code != 400 {
		t.Fatal("managed route accepted a customer hostname", w.Code)
	}
}

func TestManagedProtocolUsesGeneratedIdentityAndClosedRegistration(t *testing.T) {
	d := managedFixture(t)
	retainManagedReceipt(t, d, managedFixtureID)
	host, _ := managednamespace.Endpoint(managedFixtureID)
	environment := strings.Join(d.runtimeEnvironment(managedFixtureID, secrets{}), "\n") + "\n"
	for _, value := range []string{"PDS_HOSTNAME=" + host, "PDS_SERVICE_DID=did:web:" + host, "PDS_DEV_MODE=false", "PDS_SERVICE_HANDLE_DOMAINS=.subcult.tv", "PDS_DID_PLC_URL=https://plc.directory", "PDS_INVITE_REQUIRED=true"} {
		if !strings.Contains(environment, value+"\n") {
			t.Fatal("missing public protocol policy", value)
		}
	}
	if strings.Contains(environment, "PDS_EMAIL_") || strings.Contains(environment, ".localhost") || strings.Contains(environment, "fixtures") {
		t.Fatal("managed runtime acquired private identity or mail configuration")
	}
	configuration := d.ingressConfiguration(managedFixtureID)
	if !strings.Contains(configuration, "@allowed host "+managedFixtureID+".localhost "+host) || !strings.Contains(configuration, "Account creation is closed") || !strings.Contains(configuration, "Unknown PDS host") {
		t.Fatal("managed ingress did not retain exact hosts and closed registration")
	}
	if _, err := pilotRuntime(t, t.TempDir()).EnsureManaged(context.Background(), managedFixtureID); err == nil {
		t.Fatal("managed reconciliation worked without opt-in")
	}
}
