package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestLocalIdentityEnvironmentAndCapacityReservation(t *testing.T) {
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	runtime := &Docker{}
	if slices.Contains(runtime.runtimeEnvironment(id, secrets{}), "PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582") {
		t.Fatal("identity fixtures enabled implicitly")
	}
	runtime.EnableLocalIdentityFixtures()
	if !slices.Contains(runtime.runtimeEnvironment(id, secrets{}), "PDS_DID_PLC_URL=http://pds-ass-protocol-fixtures:2582") || !slices.Contains(runtime.runtimeEnvironment(id, secrets{}), "PDS_EMAIL_SMTP_URL=smtp://pds-ass-protocol-fixtures:2525") {
		t.Fatal("isolated identity transport missing")
	}
	config := runtime.ingressConfiguration(id)
	if !strings.Contains(config, "forward_auth @registration pds-ass-enrollment-"+id+":2727") || !strings.Contains(config, "@atproto/oauth-provider/~api/sign-up") || strings.Contains(config, `\n`) {
		t.Fatal("registration route lacks admission guard")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			_ = json.NewEncoder(w).Encode(map[string]any{"NCPU": 8, "MemTotal": int64(2<<30) + 896<<20})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"Labels": map[string]string{label: id, roleLabel: "enrollment-guard"}}})
	}))
	defer server.Close()
	runtime.client = &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
	capacity, err := runtime.Capacity(context.Background())
	if err != nil || capacity.MaximumSlots != 0 || len(capacity.InstanceIDs) != 0 {
		t.Fatalf("guard resource reservation: %+v %v", capacity, err)
	}
}

func TestLocalIdentityRefusesForeignProtocolFixture(t *testing.T) {
	for _, value := range []map[string]any{
		{"Config": map[string]any{"Image": "foreign"}},
		{"Config": map[string]any{"Image": PDSImage, "Labels": map[string]string{"com.docker.compose.project": "pds-ass-runtime", "com.docker.compose.service": "protocol-fixtures"}, "Cmd": []string{"node", "/fixtures/protocol-services.mjs"}}, "State": map[string]bool{"Running": false}},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Error("foreign fixture caused host mutation")
			}
			_ = json.NewEncoder(w).Encode(value)
		}))
		runtime := &Docker{client: &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}}
		if err := runtime.connectLocalIdentity(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); err == nil {
			t.Fatal("foreign fixture accepted")
		}
		server.Close()
	}
}
