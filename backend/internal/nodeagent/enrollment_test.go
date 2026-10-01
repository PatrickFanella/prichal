package nodeagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrollmentReplaysLostResponseWithoutExtendingLeaseOrAddingAccount(t *testing.T) {
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	ticket := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	runtime.EnableLocalIdentityFixtures()
	secret, err := runtime.credentials(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.localCertificate(id); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-edge", "-enrollment"} {
		if err = os.Mkdir(filepath.Join(directory, id+suffix), 0700); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(directory, id+"-edge", "Caddyfile")
	tlsPath := filepath.Join(directory, id+"-tls.pem")
	if err = os.WriteFile(configPath, []byte(runtime.ingressConfiguration(id)), 0600); err != nil {
		t.Fatal(err)
	}
	network := "pds-ass-tenant-" + id + "-network"
	pds := map[string]any{
		"Config":          map[string]any{"Image": PDSImage, "Labels": map[string]string{label: id}, "Env": runtime.runtimeEnvironment(id, secret)},
		"HostConfig":      map[string]any{"Memory": 768 << 20, "NanoCpus": 1e9, "PidsLimit": 128, "NetworkMode": network, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}},
		"Mounts":          []map[string]any{{"Type": "volume", "Name": "pds-ass-tenant-" + id + "-data", "Destination": "/pds", "RW": true}, {"Type": "volume", "Name": "pds-ass-tenant-" + id + "-blobs", "Destination": "/blobs", "RW": true}},
		"NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}}},
	}
	edge := map[string]any{
		"Config":     map[string]any{"Image": CaddyImage, "Labels": map[string]string{label: id, roleLabel: "ingress"}},
		"HostConfig": map[string]any{"Memory": 128 << 20, "NanoCpus": 250000000, "PidsLimit": 64, "NetworkMode": network, "CapDrop": []string{"ALL"}, "CapAdd": []string{"NET_BIND_SERVICE"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Binds": []string{configPath + ":/etc/caddy/Caddyfile:ro", tlsPath + ":/etc/pds-tls.pem:ro"}},
		"Mounts":     []map[string]any{{"Type": "bind", "Source": configPath, "Destination": "/etc/caddy/Caddyfile"}, {"Type": "bind", "Source": tlsPath, "Destination": "/etc/pds-tls.pem"}},
		"State":      map[string]any{"Running": true}, "NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}, "pds-ass-agent-ingress": map[string]any{}}},
	}
	enrollmentDirectory := filepath.Join(directory, id+"-enrollment")
	guard := map[string]any{
		"Config":     map[string]any{"Image": PDSImage, "Labels": map[string]string{label: id, roleLabel: "enrollment-guard"}, "Cmd": []string{"node", "--input-type=module", "-e", enrollmentGuardScript}, "Env": []string{"PDS_GUARD_INSTANCE=" + id}},
		"HostConfig": map[string]any{"Memory": 64 << 20, "NanoCpus": 125000000, "PidsLimit": 32, "ReadonlyRootfs": true, "NetworkMode": network, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}, "Binds": []string{enrollmentDirectory + ":/enrollment:ro"}},
		"Mounts":     []map[string]any{{"Type": "bind", "Source": enrollmentDirectory, "Destination": "/enrollment"}},
		"State":      map[string]any{"Running": true}, "NetworkSettings": map[string]any{"Networks": map[string]any{network: map[string]any{}}},
	}
	running := true
	helpers := map[string][]string{}
	counter, invitations, actors := 0, 0, 0
	loseInvitationResponse := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.44")
		switch {
		case r.Method == "GET" && strings.HasPrefix(path, "/containers/"):
			var value map[string]any
			switch {
			case strings.Contains(path, "/pds-ass-tenant-"):
				value = pds
				value["State"] = map[string]any{"Running": running, "Health": map[string]string{"Status": "healthy"}}
			case strings.Contains(path, "/pds-ass-edge-"):
				value = edge
			case strings.Contains(path, "/pds-ass-enrollment-"):
				value = guard
			default:
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(value)
		case strings.HasSuffix(path, "/stop"):
			running = false
			w.WriteHeader(204)
		case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/start"):
			running = true
			w.WriteHeader(204)
		case strings.HasSuffix(path, "/exec"):
			var body struct{ Cmd []string }
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid fixed helper")
			}
			counter++
			key := fmt.Sprint(counter)
			helpers[key] = body.Cmd
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": key})
		case strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/start"):
			key := strings.TrimSuffix(strings.TrimPrefix(path, "/exec/"), "/start")
			args := helpers[key]
			var value any
			if strings.Contains(args[3], "/enrollment/lease.json") {
				data, err := os.ReadFile(filepath.Join(enrollmentDirectory, "lease.json"))
				if err != nil {
					t.Error(err)
				}
				hash := sha256.Sum256(data)
				value = map[string]string{"hash": hex.EncodeToString(hash[:])}
			} else if actors == 1 {
				value = nativeEnrollment{Count: 1, DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", Handle: "fixture.test", EmailConfirmed: true}
			} else if args[len(args)-1] == "invitation" {
				invitations++
				if loseInvitationResponse {
					loseInvitationResponse = false
					w.WriteHeader(503)
					return
				}
				value = nativeEnrollment{Code: fmt.Sprintf("fixture-code-%d", invitations)}
			} else {
				value = nativeEnrollment{}
			}
			data, _ := json.Marshal(value)
			_, _ = w.Write(helperFrame(1, data))
		case strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ExitCode": 0, "Running": false})
		default:
			t.Errorf("unexpected host operation %s %s", r.Method, path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	runtime.client = &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
	runtime.healthProbe = func(context.Context, string) bool { return true }
	ctx := context.Background()
	if _, err = runtime.Enroll(ctx, id, ticket); err == nil {
		t.Fatal("lost helper response reported success")
	}
	lease, err := runtime.readEnrollmentLease(id)
	if err != nil || lease.State != "preparing" || lease.Code != "" {
		t.Fatal("failed issuance left admission open")
	}
	first, err := runtime.Enroll(ctx, id, ticket)
	if err != nil || first.State != "invitation_ready" || first.Code != "fixture-code-2" {
		t.Fatalf("recovered issuance: %+v %v", first, err)
	}
	replay, err := runtime.Enroll(ctx, id, ticket)
	if err != nil || first.Code != replay.Code || first.ExpiresAt != replay.ExpiresAt || invitations != 2 {
		t.Fatal("replay minted a code or extended its lease")
	}
	lease.ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	lease.IssuedAt = lease.ExpiresAt - 900000
	if err = runtime.publishEnrollmentLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	expired, err := runtime.Enroll(ctx, id, ticket)
	if err != nil || expired.State != "expired" || expired.Code != "" || invitations != 2 {
		t.Fatal("expired replay minted an invitation")
	}
	actors = 1
	completed, err := runtime.Enroll(ctx, id, "cccccccc-cccc-cccc-cccc-cccccccccccc")
	if err != nil || completed.State != "verified" || completed.Code != "" || invitations != 2 {
		t.Fatalf("account ceiling: %+v %v", completed, err)
	}
}
