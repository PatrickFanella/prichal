package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeReleaseRequiresInspectedImageEvidence(t *testing.T) {
	imageID := "sha256:" + strings.Repeat("a", 64)
	digest := strings.Split(PDSImage, "@")[1]
	for _, tc := range []struct {
		name, ref, id, responseID, repo string
		running, unavailable, want      bool
	}{
		{"observed", PDSImage, imageID, imageID, "ghcr.io/bluesky-social/pds@" + digest, true, false, true},
		{"different observed tag", strings.Replace(PDSImage, ":0.4@", ":0.4.999@", 1), imageID, imageID, "ghcr.io/bluesky-social/pds@" + digest, true, false, true},
		{"missing manifest digest", "ghcr.io/bluesky-social/pds:0.4", imageID, imageID, "", true, false, false},
		{"missing config ID", PDSImage, "", imageID, "", true, false, false},
		{"config ID mismatch", PDSImage, imageID, "sha256:" + strings.Repeat("b", 64), "ghcr.io/bluesky-social/pds@" + digest, true, false, false},
		{"manifest mismatch", PDSImage, imageID, imageID, "ghcr.io/bluesky-social/pds@sha256:" + strings.Repeat("b", 64), true, false, false},
		{"unreachable engine", PDSImage, imageID, imageID, "", true, true, false},
		{"stopped", PDSImage, imageID, imageID, "ghcr.io/bluesky-social/pds@" + digest, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/v1.44/images/"+imageID+"/json" {
					t.Errorf("unexpected engine request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
					return
				}
				if tc.unavailable {
					w.WriteHeader(503)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"Id": tc.responseID, "RepoDigests": []string{tc.repo}, "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": "0.4.5036"}}})
			}))
			defer server.Close()
			d := &Docker{client: &http.Client{Transport: &rewriteTransport{base: server.URL, transport: http.DefaultTransport}}}
			var current inspected
			current.Config.Image = tc.ref
			current.Image = tc.id
			current.State.Running = tc.running
			result := d.runtimeRelease(context.Background(), current)
			if (result != nil) != tc.want {
				t.Fatalf("release %+v", result)
			}
			if result != nil && (!result.Valid() || result.Version != "0.4.5036" || result.ImageRef != tc.ref || result.ImageID != imageID) {
				t.Fatal("release came from constant or invalid evidence")
			}
		})
	}
}
