package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRuntime struct{ calls int }

func (f *fakeRuntime) Ensure(context.Context, string) (Instance, error) {
	f.calls++
	return Instance{ID: "test", State: "running"}, nil
}
func TestAgentContract(t *testing.T) {
	runtime := &fakeRuntime{}
	handler := Handler(runtime)
	for _, entry := range []struct {
		name, path, body string
		status           int
	}{{"valid", "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", "{}", 200}, {"image injection", "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", `{"image":"evil"}`, 400}, {"path injection", "/v1/instances/../../host", "{}", 404}, {"unknown command", "/v1/exec", "{}", 404}, {"trailing object", "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", "{} {}", 400}} {
		t.Run(entry.name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", entry.path, strings.NewReader(entry.body))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != entry.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	if runtime.calls != 1 {
		t.Fatalf("runtime called %d times", runtime.calls)
	}
}
func TestUnknownPeerRejected(t *testing.T) {
	// HTTP is forbidden even if a caller reaches the private listener.
	handler := Authenticate(Handler(&fakeRuntime{}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/v1/instances/aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa", strings.NewReader("{}")))
	if w.Code != 401 {
		t.Fatalf("plaintext accepted: %d", w.Code)
	}
}
