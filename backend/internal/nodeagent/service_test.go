package nodeagent

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type serviceRuntime struct {
	fakeRuntime
	calls int
}

func (s *serviceRuntime) Service(_ context.Context, id string, r ServiceRequest) (ServiceResult, error) {
	s.calls++
	return ServiceResult{InstanceID: id, ServiceRequest: r}, nil
}

func TestServiceContract(t *testing.T) {
	const path = "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa/service"
	const good = `{"ticket":"bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb","generation":1,"state":"suspended"}`
	for _, entry := range []struct {
		name, path, body string
		want             int
	}{
		{"valid", path, good, 200},
		{"host injection", path, strings.TrimSuffix(good, "}") + `,"command":"rm"}`, 400},
		{"unknown state", path, strings.Replace(good, "suspended", "deleted", 1), 400},
		{"zero generation", path, strings.Replace(good, `"generation":1`, `"generation":0`, 1), 400},
		{"overflow", path, strings.Replace(good, `"generation":1`, `"generation":9007199254740993`, 1), 400},
		{"fraction", path, strings.Replace(good, `"generation":1`, `"generation":1.5`, 1), 400},
		{"missing ticket", path, `{"generation":1,"state":"running"}`, 400},
		{"trailing", path, good + "{}", 400},
		{"path traversal", "/v1/instances/../host/service", good, 404},
		{"oversized", path, good + strings.Repeat(" ", 300), 400},
	} {
		t.Run(entry.name, func(t *testing.T) {
			runtime := &serviceRuntime{}
			w := httptest.NewRecorder()
			Handler(runtime).ServeHTTP(w, httptest.NewRequest("POST", entry.path, strings.NewReader(entry.body)))
			if w.Code != entry.want {
				t.Fatalf("status %d", w.Code)
			}
			if runtime.calls != 0 && entry.want != 200 {
				t.Fatal("invalid request reached runtime")
			}
		})
	}
}

func TestServiceCustodyFailClosed(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	d := &Docker{directory: directory}
	id := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	lease := serviceLease{ServiceResult: ServiceResult{InstanceID: id, ServiceRequest: ServiceRequest{Ticket: "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb", Generation: 1, State: "running"}}, Phase: "pending"}
	if err := d.writeServiceLease(lease); err != nil {
		t.Fatal(err)
	}
	if held, err := d.serviceHeld(id); err != nil || !held {
		t.Fatal("pending resume did not fence reconciliation")
	}
	path := filepath.Join(directory, id+"-service.json")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ensure(context.Background(), id); err == nil {
		t.Fatal("unsafe custody permitted runtime access")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"instanceId":"wrong"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ensure(context.Background(), id); err == nil {
		t.Fatal("malformed custody permitted runtime access")
	}
}

func (s *serviceRuntime) Account(_ context.Context, id string, r ServiceRequest) (AccountResult, error) {
	s.calls++
	return AccountResult{ServiceResult: ServiceResult{InstanceID: id, ServiceRequest: r}, DID: "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"}, nil
}
func TestAccountBoundaryRejectsSubjectAndHostOverrides(t *testing.T) {
	const path = "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa/account"
	const good = `{"ticket":"bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb","generation":1,"state":"suspended"}`
	for _, extra := range []string{`"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"`, `"password":"PRIVATE"`, `"command":"native-delete"`} {
		runtime := &serviceRuntime{}
		w := httptest.NewRecorder()
		Handler(runtime).ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(strings.TrimSuffix(good, "}")+","+extra+"}")))
		if w.Code != 400 || runtime.calls != 0 {
			t.Fatal("unsupported moderation target reached runtime")
		}
	}
	runtime := &serviceRuntime{}
	w := httptest.NewRecorder()
	Handler(runtime).ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(good)))
	if w.Code != 200 || runtime.calls != 1 {
		t.Fatal("fixed account contract unavailable")
	}
}
