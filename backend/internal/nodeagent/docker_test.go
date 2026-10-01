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

func TestCredentialsSurviveReconstruction(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	value, err := first.credentials("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDocker("/unused.sock", directory)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := second.credentials("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if err != nil || value != replay {
		t.Fatalf("credentials changed: %v", err)
	}
	path := filepath.Join(directory, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb.json")
	if err = os.Symlink(filepath.Join(directory, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err = second.credentials("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"); err == nil {
		t.Fatal("followed credential symlink")
	}
}
func TestFixedRuntimeAndRetry(t *testing.T) {
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	created := false
	running := false
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.44")
		switch {
		case r.Method == "GET" && strings.HasPrefix(path, "/containers/"):
			if !created {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"HostConfig": map[string]any{"Memory": 768 << 20, "NanoCpus": 1e9, "PidsLimit": 128, "NetworkMode": "pds-ass-tenant-" + id + "-network", "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges:true"}}, "Mounts": []map[string]any{{"Type": "volume", "Name": "pds-ass-tenant-" + id + "-data", "Destination": "/pds", "RW": true}, {"Type": "volume", "Name": "pds-ass-tenant-" + id + "-blobs", "Destination": "/blobs", "RW": true}}, "Id": "container-id", "Config": map[string]any{"Image": PDSImage, "Labels": map[string]string{label: id}}, "State": map[string]bool{"Running": running}, "NetworkSettings": map[string]any{"Ports": map[string]any{"2583/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": "12345"}}}}})
		case r.Method == "GET":
			w.WriteHeader(404)
		case path == "/containers/create":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["Image"] != PDSImage {
				t.Error("unpinned image")
			}
			host, ok := payload["HostConfig"].(map[string]any)
			if !ok {
				t.Fatal("host configuration missing")
			}
			if host["Memory"] != float64(768<<20) || host["NanoCpus"] != float64(1e9) || host["PidsLimit"] != float64(128) {
				t.Error("resource limits missing")
			}
			if _, ok = host["Binds"]; ok {
				t.Error("host bind mount allowed")
			}
			created = true
			creates++
			w.WriteHeader(201)
		case strings.HasSuffix(path, "/start"):
			running = true
			w.WriteHeader(204)
		default:
			w.WriteHeader(201)
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	docker := &Docker{client: server.Client(), directory: directory}
	docker.client.Transport = &rewriteTransport{base: server.URL, transport: http.DefaultTransport}
	for i := 0; i < 2; i++ {
		result, err := docker.Ensure(context.Background(), id)
		if err != nil || result.State != "running" || result.Port != "12345" {
			t.Fatalf("reconciliation: %+v %v", result, err)
		}
	}
	if creates != 1 {
		t.Fatalf("duplicate containers: %d", creates)
	}
}

type rewriteTransport struct {
	base      string
	transport http.RoundTripper
}

func (r *rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	target, _ := http.NewRequest(request.Method, r.base+request.URL.RequestURI(), nil)
	clone.URL = target.URL
	return r.transport.RoundTrip(clone)
}
