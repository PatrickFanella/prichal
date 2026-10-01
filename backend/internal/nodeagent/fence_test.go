package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetainedWriterFence(t *testing.T) {
	id := "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
	for _, state := range []string{"running", "paused", "restarting", "exited", "created", "dead"} {
		t.Run(state, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var filters map[string][]string
				if json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters) != nil || len(filters["label"]) != 1 || filters["label"][0] != label+"="+id {
					t.Error("namespace query is not scoped")
				}
				_ = json.NewEncoder(w).Encode([]any{map[string]any{"Labels": map[string]string{label: id}, "State": state}})
			}))
			defer server.Close()
			docker := &Docker{client: &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}}
			err := docker.fenceRetainedNamespace(context.Background(), id)
			safe := state == "exited" || state == "created" || state == "dead"
			if (err == nil) != safe {
				t.Fatalf("fence: %v", err)
			}
		})
	}
}
