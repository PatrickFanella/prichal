package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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
	configurations := map[string]map[string]any{}
	running := map[string]bool{}
	creates := map[string]int{}
	readOnly := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if readOnly && r.Method != "GET" {
			t.Error("inspection mutated Docker")
			w.WriteHeader(500)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/v1.44")
		switch {
		case path == "/info":
			_ = json.NewEncoder(w).Encode(map[string]any{"NCPU": 8, "MemTotal": 8 << 30})
		case path == "/containers/json":
			_ = json.NewEncoder(w).Encode([]any{})
		case r.Method == "GET" && strings.HasPrefix(path, "/containers/"):
			name := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
			payload, exists := configurations[name]
			if !exists {
				w.WriteHeader(404)
				return
			}
			host := payload["HostConfig"].(map[string]any)
			mounts := []map[string]any{}
			if values, ok := host["Mounts"].([]any); ok {
				for _, value := range values {
					mount := value.(map[string]any)
					mounts = append(mounts, map[string]any{"Type": mount["Type"], "Name": mount["Source"], "Destination": mount["Target"], "RW": true})
				}
			}
			if values, ok := host["Binds"].([]any); ok {
				for _, value := range values {
					parts := strings.Split(value.(string), ":")
					mounts = append(mounts, map[string]any{"Type": "bind", "Source": parts[0], "Destination": parts[1], "RW": false})
				}
			}
			slices.Reverse(mounts) // Engine inspection does not promise mount order.
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": name, "Config": payload, "HostConfig": host, "Mounts": mounts, "State": map[string]any{"Running": running[name], "Health": map[string]string{"Status": "healthy"}}, "NetworkSettings": map[string]any{"Networks": func() map[string]any {
				if config, ok := payload["NetworkingConfig"].(map[string]any); ok {
					return config["EndpointsConfig"].(map[string]any)
				}
				return map[string]any{host["NetworkMode"].(string): map[string]any{}}
			}(), "Ports": map[string]any{}}})
		case r.Method == "GET":
			w.WriteHeader(404)
		case path == "/containers/create":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			name := r.URL.Query().Get("name")
			host := payload["HostConfig"].(map[string]any)
			if strings.HasPrefix(name, "pds-ass-tenant-") {
				if payload["Image"] != PDSImage || host["Memory"] != float64(768<<20) || host["NanoCpus"] != float64(1e9) || host["PidsLimit"] != float64(128) {
					t.Error("PDS policy missing")
				}
				if _, ok := host["Binds"]; ok {
					t.Error("PDS host bind mount allowed")
				}
			} else if payload["Image"] != CaddyImage {
				t.Error("unpinned ingress image")
			}
			configurations[name] = payload
			creates[name]++
			w.WriteHeader(201)
		case strings.HasSuffix(path, "/start"):
			name := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/start")
			running[name] = true
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
	docker := &Docker{client: &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}, directory: directory}
	for i := 0; i < 2; i++ {
		result, err := docker.Ensure(context.Background(), id)
		if err != nil || result.State != "running" || result.Endpoint != LocalEndpoint(id) {
			t.Fatalf("reconciliation: %+v %v", result, err)
		}
	}
	if creates["pds-ass-tenant-"+id] != 1 || creates["pds-ass-edge-"+id] != 1 {
		t.Fatalf("duplicate resources: %+v", creates)
	}
	readOnly = true
	docker.healthProbe = func(context.Context, string) bool { return true }
	result, err := docker.Status(context.Background(), id)
	if err != nil || !result.Healthy {
		t.Fatalf("inspection: %+v %v", result, err)
	}
	running["pds-ass-tenant-"+id] = false
	result, err = docker.Status(context.Background(), id)
	if err != nil || result.Healthy || result.State != "starting" {
		t.Fatalf("stopped inspection: %+v %v", result, err)
	}
	running["pds-ass-tenant-"+id] = true
	credentialPath := filepath.Join(directory, id+".json")
	credential, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(credentialPath); err != nil {
		t.Fatal(err)
	}
	if _, err = docker.Status(context.Background(), id); err == nil {
		t.Fatal("inspection accepted missing credentials")
	}
	if _, err = os.Stat(credentialPath); !os.IsNotExist(err) {
		t.Fatal("inspection recreated credentials")
	}
	if err = os.WriteFile(credentialPath, credential, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = docker.Status(context.Background(), "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"); err == nil {
		t.Fatal("inspection created missing runtime")
	}
	readOnly = false
	// A healthy process with a different advertised endpoint cannot qualify as this PDS.
	configurations["pds-ass-tenant-"+id]["Env"] = []string{"PDS_HOSTNAME=foreign.localhost"}
	if _, err := docker.Status(context.Background(), id); err == nil {
		t.Fatal("accepted mismatched runtime identity")
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

func TestMeasuredCapacityBounds(t *testing.T) {
	for _, entry := range []struct {
		name     string
		cpus     int
		memory   int64
		expected int
	}{{"host overhead", 1, 8 << 30, 0}, {"memory exhaustion", 8, 2 << 30, 0}, {"one instance", 8, (2 << 30) + (896 << 20), 1}, {"qualification ceiling", 16, 32 << 30, 2}} {
		t.Run(entry.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/info") {
					_ = json.NewEncoder(w).Encode(map[string]any{"NCPU": entry.cpus, "MemTotal": entry.memory})
				} else {
					_ = json.NewEncoder(w).Encode([]any{})
				}
			}))
			defer server.Close()
			docker := &Docker{client: &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}}
			capacity, err := docker.Capacity(context.Background())
			if err != nil || capacity.MaximumSlots != entry.expected {
				t.Fatalf("capacity: %+v %v", capacity, err)
			}
		})
	}
}

func TestOperatorCapacityStillReservesMeasuredResources(t *testing.T) {
	for _, entry := range []struct {
		name              string
		cpus              int
		memory            int64
		ceiling, expected int
	}{{"three measured tenants", 8, 8 << 30, 3, 3}, {"CPU remains bounded", 3, 8 << 30, 8, 1}, {"memory remains bounded", 8, 2<<30 + 960<<20, 8, 1}, {"operator reduction", 8, 8 << 30, 1, 1}} {
		t.Run(entry.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/info") {
					_ = json.NewEncoder(w).Encode(map[string]any{"NCPU": entry.cpus, "MemTotal": entry.memory})
				} else {
					_ = json.NewEncoder(w).Encode([]any{})
				}
			}))
			defer server.Close()
			docker := &Docker{client: &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}}
			docker.EnableLocalIdentityFixtures()
			if err := docker.SetCapacityCeiling(entry.ceiling); err != nil {
				t.Fatal(err)
			}
			capacity, err := docker.Capacity(context.Background())
			if err != nil || capacity.MaximumSlots != entry.expected {
				t.Fatal("operator ceiling bypassed measured resource policy")
			}
		})
	}
	for _, maximum := range []int{-1, 0, 9, 1000} {
		if err := (&Docker{}).SetCapacityCeiling(maximum); err == nil {
			t.Fatal("unbounded ceiling accepted")
		}
	}
}
